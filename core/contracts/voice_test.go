package contracts

import (
	"context"
	"testing"
	"time"
)

func TestVoiceContractsCompileInCore(t *testing.T) {
	var _ SpeechToText = nil
	var _ STTSession = nil
	var _ TextToSpeech = nil
	var _ TTSStream = nil

	format := AudioFormat{Encoding: AudioEncodingPCM16, SampleRateHz: 16000, Channels: 1, FrameDuration: 20 * time.Millisecond}
	frame := AudioFrame{SessionID: "s1", Format: format, PCM: []byte{1, 2, 3}, Timestamp: time.Second}
	if frame.Format.Encoding != AudioEncodingPCM16 || frame.SessionID != "s1" {
		t.Fatalf("frame = %#v", frame)
	}
}

type compileSTT struct{}

func (compileSTT) Capabilities(context.Context) (STTCapabilities, error) {
	return STTCapabilities{}, nil
}
func (compileSTT) OpenSTTSession(context.Context, STTOptions) (STTSession, error) {
	return compileSTTSession{}, nil
}

type compileSTTSession struct{}

func (compileSTTSession) WriteAudio(context.Context, AudioFrame) error { return nil }
func (compileSTTSession) Events() <-chan TranscriptEvent               { return make(chan TranscriptEvent) }
func (compileSTTSession) Close(context.Context) error                  { return nil }

type compileTTS struct{}

func (compileTTS) Capabilities(context.Context) (TTSCapabilities, error) {
	return TTSCapabilities{}, nil
}
func (compileTTS) Synthesize(context.Context, SynthesisRequest) (TTSStream, error) {
	return compileTTSStream{}, nil
}

type compileTTSStream struct{}

func (compileTTSStream) Events() <-chan TTSEvent     { return make(chan TTSEvent) }
func (compileTTSStream) Close(context.Context) error { return nil }
