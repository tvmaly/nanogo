package voice

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
)

func TestVoiceSessionDoesNotSubmitPartialTranscript(t *testing.T) {
	session, agent, _, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptPartial, Text: "what"},
	)
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 0 {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
}

func TestVoiceSessionSubmitsFinalTranscriptExactlyOnce(t *testing.T) {
	session, agent, _, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "What is 2 plus 2?"},
	)
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 1 || agent.requests[0].Text != "What is 2 plus 2?" {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
}

func TestVoiceSessionSynthesizesAgentResponseWhenAutoSpeakEnabled(t *testing.T) {
	session, _, tts, sink := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "question"},
	)
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{AutoSpeakFinalResponses: boolPtr(true), TTSEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tts.Requests) != 1 || tts.Requests[0].Text != "The answer is four." {
		t.Fatalf("tts requests = %#v", tts.Requests)
	}
	if len(sink.events) == 0 {
		t.Fatal("sink did not receive TTS events")
	}
}

func TestVoiceSessionSkipsTTSWhenAutoSpeakDisabled(t *testing.T) {
	session, agent, tts, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "question"},
	)
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{AutoSpeakFinalResponses: boolPtr(false), TTSEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 1 {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
	if len(tts.Requests) != 0 {
		t.Fatalf("tts requests = %#v", tts.Requests)
	}
}

func TestVoiceSessionCanToggleSTTOffAndOnWithoutChangingAgentSession(t *testing.T) {
	session, agent, _, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"},
	)
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{STTEnabled: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err == nil {
		t.Fatal("expected disabled STT error")
	}
	if len(agent.requests) != 0 {
		t.Fatalf("agent requests = %#v", agent.requests)
	}

	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{STTEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 1 || agent.requests[0].SessionID != "s1" {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
}

func TestVoiceSessionToggleTTSOffStopsAutoSpeakAndClosesActiveStream(t *testing.T) {
	session, _, tts, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "question"},
	)
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{TTSEnabled: boolPtr(false), AutoSpeakFinalResponses: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tts.Requests) != 0 {
		t.Fatalf("tts requests = %#v", tts.Requests)
	}
}

func TestVoiceSessionToggleTTSEnablesAutoSpeakWhenConfigured(t *testing.T) {
	session, _, tts, _ := newTestSession(t,
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "q1"},
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "q2"},
	)
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{TTSEnabled: boolPtr(false), AutoSpeakFinalResponses: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if len(tts.Requests) != 0 {
		t.Fatalf("tts requests before enable = %#v", tts.Requests)
	}
	if _, err := session.UpdateVoiceState(context.Background(), VoiceStatePatch{TTSEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := session.ProcessAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tts.Requests) != 1 {
		t.Fatalf("tts requests after enable = %#v", tts.Requests)
	}
}

func newTestSession(t *testing.T, events ...contracts.TranscriptEvent) (*VoiceSession, *fakeAgent, *contractfake.TextToSpeech, *fakeSink) {
	t.Helper()
	stt := contractfake.NewSpeechToText(events...)
	tts := contractfake.NewTextToSpeech()
	agent := &fakeAgent{result: VoiceAgentResult{Text: "The answer is four."}}
	sink := &fakeSink{}
	session, err := NewSession(SessionConfig{
		SessionID: "s1",
		STT:       stt,
		TTS:       tts,
		Agent:     agent,
		Sink:      sink,
		State:     VoiceState{STTEnabled: true, TTSEnabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, agent, tts, sink
}

type fakeAgent struct {
	result   VoiceAgentResult
	requests []VoiceAgentRequest
}

func (f *fakeAgent) SubmitVoiceText(_ context.Context, req VoiceAgentRequest) (VoiceAgentResult, error) {
	f.requests = append(f.requests, req)
	return f.result, nil
}

type fakeSink struct {
	events []contracts.TTSEvent
	closed bool
}

func (f *fakeSink) WriteTTS(_ context.Context, event contracts.TTSEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeSink) Close(context.Context) error {
	f.closed = true
	return nil
}

func boolPtr(v bool) *bool { return &v }
