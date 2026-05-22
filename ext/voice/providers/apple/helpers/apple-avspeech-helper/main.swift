import AVFoundation
import Foundation

struct AudioFormat: Codable {
    var encoding: String?
    var sample_rate_hz: Int?
    var channels: Int?
}

struct Request: Codable {
    var type: String
    var session_id: String?
    var text: String?
    var locale: String?
    var voice_id: String?
    var rate: Double?
    var pitch: Double?
    var volume: Double?
    var format: AudioFormat?
}

struct Event: Codable {
    var type: String
    var session_id: String?
    var audio_b64: String?
    var error: String?
    var sequence: Int64?
    var format: AudioFormat?
}

let decoder = JSONDecoder()
let encoder = JSONEncoder()

func emit(_ event: Event) {
    if let data = try? encoder.encode(event), let line = String(data: data, encoding: .utf8) {
        print(line)
        fflush(stdout)
    }
}

func pcm16Base64(from buffer: AVAudioBuffer) -> String {
    guard let pcm = buffer as? AVAudioPCMBuffer else { return "" }
    let frames = Int(pcm.frameLength)
    if let int16 = pcm.int16ChannelData {
        let data = Data(bytes: int16[0], count: frames * MemoryLayout<Int16>.size)
        return data.base64EncodedString()
    }
    guard let floats = pcm.floatChannelData else { return "" }
    var out = Data(capacity: frames * MemoryLayout<Int16>.size)
    for i in 0..<frames {
        let clamped = max(-1.0, min(1.0, floats[0][i]))
        var sample = Int16(clamped * Float(Int16.max))
        out.append(Data(bytes: &sample, count: MemoryLayout<Int16>.size))
    }
    return out.base64EncodedString()
}

func synthesize(_ req: Request) {
    let sessionID = req.session_id
    guard let text = req.text, !text.isEmpty else {
        emit(Event(type: "error", session_id: sessionID, error: "text is required", sequence: 1))
        return
    }

    let utterance = AVSpeechUtterance(string: text)
    if let locale = req.locale, !locale.isEmpty {
        utterance.voice = AVSpeechSynthesisVoice(language: locale)
    }
    if let voiceID = req.voice_id, !voiceID.isEmpty {
        utterance.voice = AVSpeechSynthesisVoice(identifier: voiceID)
    }
    if let rate = req.rate, rate > 0 {
        utterance.rate = Float(rate)
    }
    if let pitch = req.pitch, pitch > 0 {
        utterance.pitchMultiplier = Float(pitch)
    }
    if let volume = req.volume, volume > 0 {
        utterance.volume = Float(volume)
    }

    let synthesizer = AVSpeechSynthesizer()
    var seq: Int64 = 1
    var done = false
    emit(Event(type: "started", session_id: sessionID, sequence: seq))
    synthesizer.write(utterance) { buffer in
        if let pcm = buffer as? AVAudioPCMBuffer, pcm.frameLength == 0 {
            seq += 1
            emit(Event(type: "done", session_id: sessionID, sequence: seq))
            done = true
            return
        }
        let audio = pcm16Base64(from: buffer)
        if !audio.isEmpty {
            seq += 1
            let format = AudioFormat(encoding: "pcm16", sample_rate_hz: 24000, channels: 1)
            emit(Event(type: "audio", session_id: sessionID, audio_b64: audio, sequence: seq, format: format))
        }
    }
    while !done {
        RunLoop.current.run(mode: .default, before: Date(timeIntervalSinceNow: 0.05))
    }
}

while let line = readLine() {
    do {
        let req = try decoder.decode(Request.self, from: Data(line.utf8))
        if req.type == "synthesize" {
            synthesize(req)
        } else if req.type == "close" {
            exit(0)
        } else {
            emit(Event(type: "error", session_id: req.session_id, error: "unknown request type \(req.type)"))
        }
    } catch {
        emit(Event(type: "error", error: "decode request: \(error)"))
    }
}
