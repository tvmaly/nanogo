package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

func TestStartObsJSONLDriverRecordsEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := &config{}
	cfg.Obs = append(cfg.Obs, struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	}{
		Driver: "jsonl",
		Config: json.RawMessage(`{"root":"` + filepath.ToSlash(dir) + `","flush_on_run_finish":true}`),
	})
	bus := event.NewBus()
	stop, err := startObs(context.Background(), bus, cfg)
	if err != nil {
		t.Fatalf("startObs: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	bus.Publish(event.Event{Kind: event.TurnStarted, Session: "s1", At: time.Unix(10, 0).UTC()})
	bus.Publish(event.Event{Kind: event.TurnCompleted, Session: "s1", At: time.Unix(11, 0).UTC(), Payload: event.TurnCompletedPayload{Model: "m1", InputTokens: 1}})
	stop()

	data, err := os.ReadFile(filepath.Join(dir, "observations.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"type":"run.start"`) || !strings.Contains(string(data), `"type":"run.finish"`) {
		t.Fatalf("data = %s", data)
	}
}

func TestLoadConfigAcceptsJSONLObsDriver(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}},"obs":[{"driver":"jsonl","config":{"root":".nanogo/obs"}}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
}
