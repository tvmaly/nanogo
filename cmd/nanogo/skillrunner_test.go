package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/skills"
)

func parseJSON(data json.RawMessage, v any) error { return json.Unmarshal(data, v) }

// TestSkillRunner_SetsCtxKeySkill verifies that cliSkillRunner sets llm.CtxKeySkill
// on the context before the first LLM call, enabling router dispatch by skill name.
func TestSkillRunner_SetsCtxKeySkill(t *testing.T) {
	t.Parallel()

	var gotSkill string
	provider := fakellm.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		gotSkill, _ = ctx.Value(llm.CtxKeySkill).(string)
		ch := make(chan llm.Chunk, 2)
		ch <- llm.Chunk{TextDelta: "result"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})

	bus := event.NewBus()
	store := session.NewStore(t.TempDir(), nil)
	runner := &cliSkillRunner{provider: provider, store: store, bus: bus}

	_, err := runner.RunSkill(context.Background(), skills.RunSkillOpts{
		SkillName: "my-skill",
		UserMsg:   "hello",
	})
	if err != nil {
		t.Fatalf("RunSkill: %v", err)
	}
	if gotSkill != "my-skill" {
		t.Errorf("CtxKeySkill = %q, want %q", gotSkill, "my-skill")
	}
}

// TestSkillRunner_ToolsAllowlist verifies that when RunSkillOpts.Tools is non-empty,
// only those tools are available in the subagent execution.
func TestSkillRunner_ToolsAllowlist(t *testing.T) {
	t.Parallel()

	var gotTools []string
	provider := fakellm.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		for _, ts := range req.Tools {
			var schema struct {
				Function struct{ Name string } `json:"function"`
			}
			if err := parseJSON(ts, &schema); err == nil {
				gotTools = append(gotTools, schema.Function.Name)
			}
		}
		ch := make(chan llm.Chunk, 2)
		ch <- llm.Chunk{TextDelta: "ok"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})

	bus := event.NewBus()
	store := session.NewStore(t.TempDir(), nil)
	runner := &cliSkillRunner{provider: provider, store: store, bus: bus}

	_, err := runner.RunSkill(context.Background(), skills.RunSkillOpts{
		SkillName: "restricted-skill",
		UserMsg:   "do it",
		Tools:     []string{"read_file"},
	})
	if err != nil {
		t.Fatalf("RunSkill: %v", err)
	}

	for _, name := range gotTools {
		if name != "read_file" {
			t.Errorf("tool %q was available but not in allowlist", name)
		}
	}
	found := false
	for _, name := range gotTools {
		if name == "read_file" {
			found = true
		}
	}
	if !found {
		t.Error("allowed tool read_file not available")
	}

	// bash must NOT be present
	for _, name := range gotTools {
		if strings.EqualFold(name, "bash") {
			t.Error("bash was available but not in allowlist")
		}
	}
}
