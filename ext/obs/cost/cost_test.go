package cost_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/ext/obs/cost"
)

func TestCostPricing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cost.jsonl")
	tracker := cost.New(cost.Config{
		OutputPath: path,
		Prices: map[string]cost.Price{
			"model-a": {InputPerMTok: 1, OutputPerMTok: 5, CachedInputPerMTok: 0.1},
		},
	})
	tracker.Record(context.Background(), event.Event{
		Kind:    event.TurnCompleted,
		Session: "s1",
		Turn:    2,
		At:      time.Unix(10, 0).UTC(),
		Payload: event.TurnCompletedPayload{
			Model: "model-a", InputTokens: 1000, OutputTokens: 500, CachedInputTokens: 200,
			Source: "cli", Skill: "lesson",
		},
	})
	rec := readOne(t, path)
	if rec["cost_usd"].(float64) != 0.00332 {
		t.Fatalf("cost = %.6f", rec["cost_usd"].(float64))
	}
	if rec["source"] != "cli" || rec["skill"] != "lesson" {
		t.Fatalf("metadata = %+v", rec)
	}
}

func TestCostUnknownModel(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cost.jsonl")
	tracker := cost.New(cost.Config{OutputPath: path, Prices: map[string]cost.Price{}})
	tracker.Record(context.Background(), event.Event{
		Kind:    event.TurnCompleted,
		Payload: event.TurnCompletedPayload{Model: "missing", InputTokens: 1},
	})
	rec := readOne(t, path)
	if rec["cost_usd"] != nil || rec["error"] != "unknown_model" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestCostMissingUsage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cost.jsonl")
	tracker := cost.New(cost.Config{
		OutputPath: path,
		Prices:     map[string]cost.Price{"model-a": {InputPerMTok: 1}},
	})
	tracker.Record(context.Background(), event.Event{
		Kind:    event.TurnCompleted,
		Payload: event.TurnCompletedPayload{Model: "model-a"},
	})
	rec := readOne(t, path)
	if rec["cost_usd"] != nil || rec["error"] != "no_usage" {
		t.Fatalf("record = %+v", rec)
	}
}

func readOne(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("json: %v: %s", err, raw)
	}
	return rec
}

// TEST-10.14 — cost adapter preserves server_tool_use in JSONL output
func TestCostServerToolUsage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cost.jsonl")
	tracker := cost.New(cost.Config{
		OutputPath: path,
		Prices:     map[string]cost.Price{"m": {InputPerMTok: 1.0, OutputPerMTok: 5.0}},
	})

	e := event.Event{
		Kind: event.TurnCompleted,
		At:   time.Now(),
		Payload: event.TurnCompletedPayload{
			Model:        "m",
			InputTokens:  100,
			OutputTokens: 50,
			Source:       "cli",
			ServerToolUse: map[string]int{"web_search_requests": 2},
		},
	}
	if err := tracker.Record(context.Background(), e); err != nil {
		t.Fatalf("Record: %v", err)
	}

	raw, _ := os.ReadFile(path)
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("json: %v", err)
	}
	stu, ok := rec["server_tool_use"].(map[string]any)
	if !ok {
		t.Fatalf("server_tool_use missing from cost record, got: %v", rec)
	}
	if int(stu["web_search_requests"].(float64)) != 2 {
		t.Errorf("expected web_search_requests=2, got %v", stu["web_search_requests"])
	}
}
