package session

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
	"github.com/tvmaly/nanogo/ext/voice/realtime/fake"
)

func TestVoiceSessionLifecycle(t *testing.T) {
	adapter := fake.New(realtime.Event{Type: realtime.EventResponseDone})
	mgr := NewManager(Config{
		Workspace:     t.TempDir(),
		Provider:      adapter,
		SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate},
	})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("missing session ID")
	}
	if len(adapter.Conn.Sent) == 0 || adapter.Conn.Sent[0].Type != realtime.EventSessionUpdate {
		t.Fatalf("sent = %#v", adapter.Conn.Sent)
	}
	if err := mgr.Close(s.ID); err != nil {
		t.Fatal(err)
	}
}

func TestVoiceTextOnlyTurn(t *testing.T) {
	adapter := fake.New(realtime.Event{Type: realtime.EventResponseTextDelta, Text: "hello"})
	mgr := NewManager(Config{Workspace: t.TempDir(), Provider: adapter, SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate}})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, _ := mgr.Events(s.ID)
	if err := mgr.TextSend(context.Background(), s.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ResponseCreate(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-events:
		if got.Type != realtime.EventResponseTextDelta || got.Text != "hello" {
			t.Fatalf("event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestVoiceAudioBufferOps(t *testing.T) {
	adapter := fake.New()
	mgr := NewManager(Config{Workspace: t.TempDir(), Provider: adapter, SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate}})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	audio := base64.StdEncoding.EncodeToString([]byte{0, 1, 2})
	if err := mgr.AudioAppend(context.Background(), s.ID, audio); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AudioCommit(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AudioClear(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AudioAppend(context.Background(), s.ID, "not base64"); err == nil {
		t.Fatal("expected invalid base64 error")
	}
}

func TestVoiceSessionEventsAPI(t *testing.T) {
	adapter := fake.New(realtime.Event{Type: realtime.EventResponseTextDelta, Text: "ok"})
	mgr := NewManager(Config{Workspace: t.TempDir(), Provider: adapter, SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate}})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, err := mgr.Events(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-events:
		if got.SessionID != s.ID || got.Provider != "fake" {
			t.Fatalf("event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestVoiceSessionPersistence(t *testing.T) {
	dir := t.TempDir()
	adapter := fake.New(realtime.Event{Type: realtime.EventResponseTextDelta, Text: "ok", Raw: []byte(`{"type":"response.text.delta"}`)})
	mgr := NewManager(Config{Workspace: dir, Provider: adapter, SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate}})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, _ := mgr.Events(s.ID)
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	if _, err := os.Stat(filepath.Join(dir, "memory", "voice", s.ID+"_events.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "memory", "voice", s.ID+"_raw.jsonl")); err == nil {
		t.Fatal("raw provider log should be opt-in")
	}
}

func TestVoiceRawProviderPersistenceRequiresOptIn(t *testing.T) {
	dir := t.TempDir()
	adapter := fake.New(realtime.Event{Type: realtime.EventResponseTextDelta, Text: "ok", Raw: []byte(`{"secret":"provider payload"}`)})
	mgr := NewManager(Config{Workspace: dir, Provider: adapter, PersistRawProviderEvents: true, SessionUpdate: realtime.Event{Type: realtime.EventSessionUpdate}})
	s, err := mgr.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, _ := mgr.Events(s.ID)
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	if _, err := os.Stat(filepath.Join(dir, "memory", "voice", s.ID+"_raw.jsonl")); err != nil {
		t.Fatal(err)
	}
}
