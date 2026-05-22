package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	"github.com/tvmaly/nanogo/ext/voice/localaudio"
	voice "github.com/tvmaly/nanogo/modules/voice"
)

func TestVoiceProvidersCommandListsConfiguredProviders(t *testing.T) {
	var out bytes.Buffer
	err := runVoiceProviders(context.Background(), &out, voiceProviderRegistry{
		STT: map[string]contracts.SpeechToText{"fake": contractfake.NewSpeechToText()},
		TTS: map[string]contracts.TextToSpeech{"fake": contractfake.NewTextToSpeech()},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "stt fake") || !strings.Contains(got, "tts fake") {
		t.Fatalf("output = %q", got)
	}
}

func TestVoiceSTTFileCommandPrintsFinalTranscript(t *testing.T) {
	var out bytes.Buffer
	provider := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"})
	if err := runVoiceSTT(context.Background(), &out, provider, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestVoiceTTSCommandWritesToSink(t *testing.T) {
	sink := &recordingSink{}
	if err := runVoiceTTS(context.Background(), contractfake.NewTextToSpeech(), sink, "hello"); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) == 0 {
		t.Fatal("expected tts events")
	}
}

type recordingSink struct {
	events []contracts.TTSEvent
}

func (s *recordingSink) WriteTTS(_ context.Context, event contracts.TTSEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingSink) Close(context.Context) error { return nil }

func TestVoiceChatRoutesFinalTranscriptToAgentAndTTS(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	stt := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "what is a magnet"})
	tts := contractfake.NewTextToSpeech()
	agent := &fakeVoiceAgent{reply: "A magnet pulls some metals."}

	if err := runVoiceChat(context.Background(), capture, playback, stt, tts, agent, "en-US", voiceChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 1 || agent.requests[0].Text != "what is a magnet" {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
	if string(playback.Writes()[0]) != "A magnet pulls some metals." {
		t.Fatalf("playback writes = %#v", playback.Writes())
	}
}

func TestVoiceChatDebugReportsPipelineStages(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	stt := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"})
	tts := contractfake.NewTextToSpeech()
	agent := &fakeVoiceAgent{reply: "hi"}
	var out bytes.Buffer

	if err := runVoiceChat(context.Background(), capture, playback, stt, tts, agent, "en-US", voiceChatOptions{DebugOut: &out}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"capture bytes=", `tts audio bytes=2`, "tts done"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug output = %q, missing %q", got, want)
		}
	}
}

func TestDebouncedSpeechToTextEmitsLatestFinalAfterIdle(t *testing.T) {
	inner := contractfake.NewSpeechToText(
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "Can"},
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "Can you"},
	)
	wrapped := debouncedSpeechToText{inner: inner, delay: 10 * time.Millisecond}
	session, err := wrapped.OpenSTTSession(context.Background(), contracts.STTOptions{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{PCM: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-session.Events():
		if event.Text != "Can you" {
			t.Fatalf("event text = %q", event.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced transcript")
	}
}

type fakeVoiceAgent struct {
	reply    string
	requests []voice.VoiceAgentRequest
}

func (a *fakeVoiceAgent) SubmitVoiceText(_ context.Context, req voice.VoiceAgentRequest) (voice.VoiceAgentResult, error) {
	a.requests = append(a.requests, req)
	return voice.VoiceAgentResult{Text: a.reply}, nil
}
