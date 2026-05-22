package speechanalyzer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/voice/providers/apple/helper"
)

func TestOpenSessionMapsHelperTranscriptEvents(t *testing.T) {
	provider := NewWithHelper(writeHelper(t, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"type":"audio"'*)
      printf '%s\n' '{"type":"partial","session_id":"stt1","text":"hel","sequence":1}'
      printf '%s\n' '{"type":"final","session_id":"stt1","text":"hello","locale":"en-US","confidence":0.8,"sequence":2}'
      ;;
    *'"type":"close"'*)
      exit 0
      ;;
  esac
done
`))
	session, err := provider.OpenSTTSession(context.Background(), contracts.STTOptions{SessionID: "stt1", Locale: "en-US", InterimResults: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{PCM: []byte{1, 2}}); err != nil {
		t.Fatal(err)
	}
	var events []contracts.TranscriptEvent
	for len(events) < 2 {
		events = append(events, <-session.Events())
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if events[0].Kind != contracts.TranscriptPartial || events[1].Kind != contracts.TranscriptFinal || events[1].Text != "hello" {
		t.Fatalf("events = %#v", events)
	}
}

func TestMissingHelperReturnsUnavailable(t *testing.T) {
	provider := NewWithHelper(filepath.Join(t.TempDir(), "missing-helper"))
	_, err := provider.OpenSTTSession(context.Background(), contracts.STTOptions{SessionID: "stt1"})
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
