package fake

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
)

func TestFakeSTTSessionEmitsPartialAndFinalInOrder(t *testing.T) {
	provider := NewSpeechToText(
		contracts.TranscriptEvent{Kind: contracts.TranscriptPartial, Text: "hel"},
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"},
	)
	session, err := provider.OpenSTTSession(context.Background(), contracts.STTOptions{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{SessionID: "s1", PCM: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	var got []contracts.TranscriptEvent
	for event := range session.Events() {
		got = append(got, event)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 || got[1].Text != "hello" {
		t.Fatalf("events = %#v", got)
	}
}

func TestFakeTTSStreamEmitsStartedAudioDoneInOrder(t *testing.T) {
	provider := NewTextToSpeech()
	stream, err := provider.Synthesize(context.Background(), contracts.SynthesisRequest{SessionID: "s1", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []contracts.TTSEvent
	for event := range stream.Events() {
		got = append(got, event)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}
	if got[0].Kind != contracts.TTSEventStarted || got[1].Kind != contracts.TTSEventAudio || got[2].Kind != contracts.TTSEventDone {
		t.Fatalf("events = %#v", got)
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 || got[2].Sequence != 3 {
		t.Fatalf("sequences = %#v", got)
	}
}
