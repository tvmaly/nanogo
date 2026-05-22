import AVFoundation
import Foundation
import Speech

struct AudioFormat: Codable {
    var encoding: String?
    var sample_rate_hz: Int?
    var channels: Int?
}

struct Request: Codable {
    var type: String
    var session_id: String?
    var locale: String?
    var interim_results: Bool?
    var audio_b64: String?
    var final: Bool?
    var format: AudioFormat?
}

struct Event: Codable {
    var type: String
    var session_id: String?
    var text: String?
    var locale: String?
    var confidence: Double?
    var error: String?
    var sequence: Int64?
}

let decoder = JSONDecoder()
let encoder = JSONEncoder()

func emit(_ event: Event) {
    if let data = try? encoder.encode(event), let line = String(data: data, encoding: .utf8) {
        print(line)
        fflush(stdout)
    }
}

@available(macOS 26.0, *)
actor AnalyzerSession {
    let sessionID: String
    let localeID: String
    let builder: AsyncStream<AnalyzerInput>.Continuation
    let inputFormat: AVAudioFormat
    var sequence: Int64 = 0

    init(sessionID: String, localeID: String) async throws {
        self.sessionID = sessionID
        self.localeID = localeID
        guard let locale = await SpeechTranscriber.supportedLocale(equivalentTo: Locale(identifier: localeID)) else {
            throw NSError(domain: "nanogo.apple.speech", code: 1, userInfo: [NSLocalizedDescriptionKey: "unsupported locale \(localeID)"])
        }
        let transcriber = SpeechTranscriber(locale: locale, preset: .progressiveTranscription)
        if let request = try await AssetInventory.assetInstallationRequest(supporting: [transcriber]) {
            try await request.downloadAndInstall()
        }
        guard let format = await SpeechAnalyzer.bestAvailableAudioFormat(compatibleWith: [transcriber]) else {
            throw NSError(domain: "nanogo.apple.speech", code: 2, userInfo: [NSLocalizedDescriptionKey: "no compatible audio format"])
        }
        self.inputFormat = format
        let stream = AsyncStream.makeStream(of: AnalyzerInput.self)
        self.builder = stream.continuation
        let analyzer = SpeechAnalyzer(modules: [transcriber])
        Task {
            do {
                for try await result in transcriber.results {
                    self.emitResult(result)
                }
            } catch {
                emit(Event(type: "error", session_id: sessionID, error: "speech results: \(error)"))
            }
        }
        Task {
            do {
                _ = try await analyzer.analyzeSequence(stream.stream)
            } catch {
                emit(Event(type: "error", session_id: sessionID, error: "analyze sequence: \(error)"))
            }
        }
    }

    func appendBase64PCM(_ encoded: String) {
        guard let data = Data(base64Encoded: encoded) else {
            emit(Event(type: "error", session_id: sessionID, error: "invalid base64 audio"))
            return
        }
        let sampleCount = data.count / MemoryLayout<Int16>.size
        guard sampleCount > 0,
              let buffer = AVAudioPCMBuffer(pcmFormat: inputFormat, frameCapacity: AVAudioFrameCount(sampleCount)) else {
            return
        }
        buffer.frameLength = AVAudioFrameCount(sampleCount)
        data.withUnsafeBytes { raw in
            let samples = raw.bindMemory(to: Int16.self)
            if let int16 = buffer.int16ChannelData {
                for i in 0..<sampleCount { int16[0][i] = samples[i] }
            } else if let floats = buffer.floatChannelData {
                for i in 0..<sampleCount { floats[0][i] = Float(samples[i]) / Float(Int16.max) }
            }
        }
        builder.yield(AnalyzerInput(buffer: buffer))
    }

    func finish() {
        builder.finish()
    }

    func emitResult(_ result: SpeechTranscriber.Result) {
        sequence += 1
        let text = String(result.text.characters)
        emit(Event(type: "final", session_id: sessionID, text: text, locale: localeID, sequence: sequence))
    }
}

var session: Any?

while let line = readLine() {
    do {
        let req = try decoder.decode(Request.self, from: Data(line.utf8))
        if #available(macOS 26.0, *) {
            switch req.type {
            case "start":
                let sid = req.session_id ?? "apple-stt"
                let locale = req.locale ?? "en-US"
                do {
                    session = try await AnalyzerSession(sessionID: sid, localeID: locale)
                } catch {
                    emit(Event(type: "error", session_id: sid, error: "\(error)"))
                }
            case "audio":
                if let analyzer = session as? AnalyzerSession {
                    await analyzer.appendBase64PCM(req.audio_b64 ?? "")
                    if req.final == true { await analyzer.finish() }
                }
            case "close":
                if let analyzer = session as? AnalyzerSession { await analyzer.finish() }
                exit(0)
            default:
                emit(Event(type: "error", session_id: req.session_id, error: "unknown request type \(req.type)"))
            }
        } else {
            emit(Event(type: "error", session_id: req.session_id, error: "SpeechAnalyzer requires macOS 26+"))
        }
    } catch {
        emit(Event(type: "error", error: "decode request: \(error)"))
    }
}
