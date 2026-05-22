package avspeech

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/voice/providers/apple/helper"
)

func TestSynthesizeMapsHelperEvents(t *testing.T) {
	provider := NewWithHelper(writeHelper(t, `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' '{"type":"started","session_id":"tts1","sequence":1}'
  printf '%s\n' '{"type":"audio","session_id":"tts1","audio_b64":"AQI=","sequence":2,"format":{"encoding":"pcm16","sample_rate_hz":24000,"channels":1}}'
  printf '%s\n' '{"type":"done","session_id":"tts1","sequence":3}'
  exit 0
done
`))
	stream, err := provider.Synthesize(context.Background(), contracts.SynthesisRequest{SessionID: "tts1", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close(context.Background())

	var events []contracts.TTSEvent
	for event := range stream.Events() {
		events = append(events, event)
		if event.Kind == contracts.TTSEventDone {
			break
		}
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[1].Kind != contracts.TTSEventAudio || string(events[1].PCM) != string([]byte{1, 2}) {
		t.Fatalf("audio event = %#v", events[1])
	}
}

func TestMissingHelperReturnsUnavailable(t *testing.T) {
	provider := NewWithHelper(filepath.Join(t.TempDir(), "missing-helper"))
	_, err := provider.Synthesize(context.Background(), contracts.SynthesisRequest{SessionID: "tts1", Text: "hello"})
	if !errors.Is(err, helper.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func writeHelper(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
