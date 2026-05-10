package xai

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
	"github.com/tvmaly/nanogo/ext/voice/realtime/fake"
)

func TestXAIConfigFromEnv(t *testing.T) {
	t.Setenv("XAI_API_KEY", "test-key")
	t.Setenv("XAI_REALTIME_MODEL", "grok-voice-think-fast-1.0")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "wss://api.x.ai/v1/realtime?model=grok-voice-think-fast-1.0" {
		t.Fatalf("url = %q", cfg.URL)
	}

	t.Setenv("XAI_API_KEY", "")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestXAISessionUpdateDefaults(t *testing.T) {
	event := SessionUpdate(Config{})
	if event.Type != realtime.EventSessionUpdate {
		t.Fatalf("type = %q", event.Type)
	}
	if event.Session.Voice != "eve" || event.Session.TurnDetection.Type != "server_vad" {
		t.Fatalf("session = %#v", event.Session)
	}
	if event.Session.AudioInput.Rate != 24000 || event.Session.AudioOutput.Type != "audio/pcm" {
		t.Fatalf("audio = %#v", event.Session)
	}
}

func TestXAIEventMapping(t *testing.T) {
	raw := []byte(`{"type":"response.text.delta","text":"hi"}`)
	event := MapWire(raw)
	if event.Type != realtime.EventResponseTextDelta || event.Text != "hi" || event.Provider != "xai" {
		t.Fatalf("event = %#v", event)
	}
}

func TestConnectUsesBearerHeader(t *testing.T) {
	conn := fake.NewConn()
	adapter := New(Config{APIKey: "test-key", Model: "m"})
	var gotURL string
	var gotHeaders map[string]string
	adapter.dial = func(_ context.Context, url string, headers map[string]string) (realtime.RealtimeConn, error) {
		gotURL = url
		gotHeaders = headers
		return conn, nil
	}
	if _, err := adapter.Connect(context.Background(), realtime.ProviderConfig{}); err != nil {
		t.Fatal(err)
	}
	if gotURL != "wss://api.x.ai/v1/realtime?model=m" {
		t.Fatalf("url = %q", gotURL)
	}
	if gotHeaders["Authorization"] != "Bearer test-key" {
		t.Fatalf("auth = %q", gotHeaders["Authorization"])
	}
}
