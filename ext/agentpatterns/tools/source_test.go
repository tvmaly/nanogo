package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	coretools "github.com/tvmaly/nanogo/core/tools"
	patterntools "github.com/tvmaly/nanogo/ext/agentpatterns/tools"
)

func TestPatternToolSourceExposesToolsAndValidatesRun(t *testing.T) {
	runtime := &contractfake.PatternRuntime{RunResult: contracts.PatternResult{Text: "ran"}}
	src := patterntools.NewSource(runtime)
	list, err := src.Tools(context.Background(), coretools.TurnInfo{Session: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list {
		names[tool.Name()] = true
	}
	for _, want := range []string{"pattern_run", "pattern_status", "pattern_resume", "pattern_list"} {
		if !names[want] {
			t.Fatalf("missing %s in %#v", want, names)
		}
	}
	_, err = list[0].Call(context.Background(), json.RawMessage(`{"pattern":"single"}`))
	if err == nil {
		t.Fatal("expected missing prompt validation error")
	}
	got, err := list[0].Call(context.Background(), json.RawMessage(`{"pattern":"single","prompt":"hi"}`))
	if err != nil || got != "ran" {
		t.Fatalf("got %q err %v", got, err)
	}
}
