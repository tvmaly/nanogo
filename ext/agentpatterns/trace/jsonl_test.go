package trace_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/agentpatterns/trace"
)

func TestJSONLSinkWritesVersionedRedactedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	sink := trace.NewJSONLSink(path, trace.Config{Redact: true})
	err := sink.EmitTrace(context.Background(), contracts.TraceEvent{
		Version: 1, RunID: "run-1", SessionID: "child-cross", Pattern: "single",
		Kind: "pattern.completed", Status: "ok", CreatedAt: time.Unix(1, 0).UTC(),
		Data: map[string]any{"api_key": "secret", "child_id": "cross", "safe": "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "cross") {
		t.Fatalf("trace not redacted: %s", data)
	}
	var rec map[string]any
	if err := json.Unmarshal(bytesTrim(data), &rec); err != nil {
		t.Fatalf("invalid jsonl: %v\n%s", err, data)
	}
	if rec["version"].(float64) != 1 || rec["pattern"] != "single" {
		t.Fatalf("record = %#v", rec)
	}
}

func bytesTrim(b []byte) []byte { return []byte(strings.TrimSpace(string(b))) }
