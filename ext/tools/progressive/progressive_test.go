package progressive_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/ext/tools/progressive"
)

func TestProgressiveHidesToolInitially(t *testing.T) {
	t.Parallel()
	ps := progressive.NewSource()
	ps.Register("advanced_search", json.RawMessage(`{"type":"function","function":{"name":"advanced_search"}}`))

	tools := ps.Tools(context.Background())
	for _, tool := range tools {
		var s map[string]any
		json.Unmarshal(tool, &s)
		if fn, ok := s["function"].(map[string]any); ok {
			if fn["name"] == "advanced_search" {
				t.Fatal("advanced_search should be hidden until revealed")
			}
		}
	}
}

func TestProgressiveRevealTool(t *testing.T) {
	t.Parallel()
	ps := progressive.NewSource()
	ps.Register("advanced_search", json.RawMessage(`{"type":"function","function":{"name":"advanced_search"}}`))

	if err := ps.RevealTool("advanced_search"); err != nil {
		t.Fatalf("RevealTool: %v", err)
	}

	found := false
	for _, tool := range ps.Tools(context.Background()) {
		var s map[string]any
		json.Unmarshal(tool, &s)
		if fn, ok := s["function"].(map[string]any); ok && fn["name"] == "advanced_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("advanced_search should be visible after RevealTool")
	}
}

func TestProgressiveRevealNonexistent(t *testing.T) {
	t.Parallel()
	ps := progressive.NewSource()
	if err := ps.RevealTool("ghost_tool"); err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}
