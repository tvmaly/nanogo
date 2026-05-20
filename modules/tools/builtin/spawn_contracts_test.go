package builtin_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/tools/builtin"
)

func TestSpawnWithoutSpawnerReturnsExistingSafeError(t *testing.T) {
	src := builtin.NewSource(nil, nil, nil)
	list, err := src.Tools(context.Background(), coretools.TurnInfo{Session: "parent"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	spawn := findTool(t, list, "spawn")

	_, err = spawn.Call(context.Background(), jsonArgs(t, map[string]any{"goal": "do work"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "spawn: no runner configured" {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestSpawnWithSubagentSpawnerAdapterInvokesSpawner(t *testing.T) {
	spawner := &capturingSpawner{result: contracts.SubagentResult{Text: "done"}}
	src := builtin.NewSource(nil, nil, coretools.NewSubagentRunnerAdapter(spawner))
	list, err := src.Tools(context.Background(), coretools.TurnInfo{Session: "parent"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	spawn := findTool(t, list, "spawn")

	got, err := spawn.Call(context.Background(), jsonArgs(t, map[string]any{
		"goal":  "review this",
		"role":  "reviewer",
		"model": "fast",
		"tools": []string{"read_file"},
	}))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got != "done" {
		t.Fatalf("result = %q", got)
	}
	if spawner.calls != 1 {
		t.Fatalf("calls = %d, want 1", spawner.calls)
	}
	if spawner.last.ParentRunID != "parent" || spawner.last.Prompt != "review this" || spawner.last.Role != "reviewer" {
		t.Fatalf("request = %+v", spawner.last)
	}
	if spawner.last.Metadata["model"] != "fast" {
		t.Fatalf("metadata = %+v", spawner.last.Metadata)
	}
	if len(spawner.last.AllowedTools) != 1 || spawner.last.AllowedTools[0] != "read_file" {
		t.Fatalf("allowed tools = %+v", spawner.last.AllowedTools)
	}
}

type capturingSpawner struct {
	calls  int
	last   contracts.SubagentRequest
	result contracts.SubagentResult
	err    error
}

func (s *capturingSpawner) SpawnSubagent(_ context.Context, req contracts.SubagentRequest) (contracts.SubagentResult, error) {
	s.calls++
	s.last = req
	return s.result, s.err
}
