package agentpatterns_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	coretools "github.com/tvmaly/nanogo/core/tools"
	faketools "github.com/tvmaly/nanogo/core/tools/fake"
	"github.com/tvmaly/nanogo/ext/agentpatterns"
)

func TestRuntimeImplementsPatternContracts(t *testing.T) {
	var _ contracts.PatternRunner = (*agentpatterns.Runtime)(nil)
	var _ contracts.PatternResumer = (*agentpatterns.Runtime)(nil)
	var _ contracts.HandoffTarget = (*agentpatterns.Runtime)(nil)
	var _ contracts.PatternRuntime = (*agentpatterns.Runtime)(nil)
}

func TestSimpleRequestDefaultsToSinglePattern(t *testing.T) {
	agent := &contractfake.AgentRunner{Result: contracts.AgentResult{Text: "practice 3/4"}}
	trace := &contractfake.TraceSink{}
	rt := agentpatterns.New(agentpatterns.Config{AgentRunner: agent, TraceSink: trace})

	got, err := rt.RunPattern(context.Background(), contracts.PatternRequest{Prompt: "Help with 1/2 + 1/4"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "practice 3/4" || len(agent.Requests) != 1 {
		t.Fatalf("result=%#v agent calls=%d", got, len(agent.Requests))
	}
	if got.Metadata["pattern"] != "single" {
		t.Fatalf("pattern = %q, want single", got.Metadata["pattern"])
	}
	if !hasTrace(trace.Events, "pattern.started", "single") || !hasTrace(trace.Events, "pattern.completed", "single") {
		t.Fatalf("trace events = %#v", trace.Events)
	}
}

func TestRouterFallsBackAndSelectsStructuredChoice(t *testing.T) {
	agent := &contractfake.AgentRunner{Result: contracts.AgentResult{Text: "ok"}}
	rt := agentpatterns.New(agentpatterns.Config{AgentRunner: agent, RouterEnabled: true})

	fallback, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "router",
		Metadata:    map[string]string{"router_choice": "???", "router_confidence": "0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Metadata["pattern"] != "single" {
		t.Fatalf("fallback pattern = %q", fallback.Metadata["pattern"])
	}

	structured, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "router",
		Metadata:    map[string]string{"router_choice": "loop", "router_confidence": "0.9", "pass_at": "2"},
		Budget:      contracts.Budget{MaxModelCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if structured.Metadata["pattern"] != "loop" || structured.Metadata["stop_reason"] != "condition_passed" {
		t.Fatalf("structured = %#v", structured.Metadata)
	}
}

func TestRuntimeUsesToolContractsAndCoreAdapter(t *testing.T) {
	toolRT := &contractfake.ToolRuntime{
		Specs:  []contracts.ToolSpec{{Name: "scratch_math"}},
		Result: contracts.ToolResult{Text: "tool answer"},
	}
	rt := agentpatterns.New(agentpatterns.Config{ToolRuntime: toolRT})

	got, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "single",
		Metadata:    map[string]string{"tool": "scratch_math"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "tool answer" || len(toolRT.Invocations) != 1 || toolRT.Invocations[0].Name != "scratch_math" {
		t.Fatalf("result=%#v invocations=%#v", got, toolRT.Invocations)
	}
}

func TestRuntimeCanUseCoreToolsContractRuntime(t *testing.T) {
	src := faketools.NewSource(faketools.New("scratch_math", "core tool answer"))
	rt := agentpatterns.New(agentpatterns.Config{ToolRuntime: coretools.NewContractRuntime(src)})
	got, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "single",
		Metadata:    map[string]string{"tool": "scratch_math"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "core tool answer" {
		t.Fatalf("result = %#v", got)
	}
}

func TestSupervisorWorkerUsesSubagentSpawnerAndFiltersContext(t *testing.T) {
	spawner := &contractfake.SubagentSpawner{Result: contracts.SubagentResult{Text: strings.Repeat("x", 120), Summary: "worker summary"}}
	rt := agentpatterns.New(agentpatterns.Config{SubagentSpawner: spawner})

	got, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "supervisor_worker",
		Context: map[string]any{
			"full_transcript":  "private",
			"private_note":     "secret",
			"compact_summary":  "allowed",
			"worker_prompts":   []any{"check gap", "make practice", "extra"},
			"allowed_tool_set": []any{"read_file"},
		},
		Budget: contracts.Budget{MaxSubagents: 2},
		Policy: contracts.PatternPolicy{
			AllowSubagents: true,
			AllowedTools:   []string{"scratch_math", "read_file"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spawner.Requests) != 2 || got.Metadata["workers"] != "2" {
		t.Fatalf("workers=%d result=%#v", len(spawner.Requests), got.Metadata)
	}
	for _, req := range spawner.Requests {
		if req.Context["full_transcript"] != nil || req.Context["private_note"] != nil {
			t.Fatalf("unfiltered context = %#v", req.Context)
		}
		if req.Context["compact_summary"] != "allowed" || len(req.AllowedTools) != 1 || req.AllowedTools[0] != "read_file" {
			t.Fatalf("request = %#v", req)
		}
	}
	if len(got.Artifacts) == 0 {
		t.Fatalf("expected compacted artifact reference, got %#v", got.Artifacts)
	}
}

func TestPatternsBudgetsHandoffApprovalResumeAndTrace(t *testing.T) {
	trace := &contractfake.TraceSink{}
	approval := &contractfake.ApprovalGate{}
	rt := agentpatterns.New(agentpatterns.Config{TraceSink: trace, ApprovalGate: approval})

	seq, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "sequential",
		Metadata:    map[string]string{"steps": "assess,explain,verify"},
	})
	if err != nil || seq.Metadata["order"] != "assess,explain,verify" {
		t.Fatalf("seq=%#v err=%v", seq.Metadata, err)
	}

	par, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "parallel",
		Metadata:    map[string]string{"branches": "a,b,c"},
		Budget:      contracts.Budget{MaxConcurrentAgents: 2},
	})
	if err != nil || par.Metadata["max_concurrent_seen"] != "2" {
		t.Fatalf("parallel=%#v err=%v", par.Metadata, err)
	}

	loop, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "loop",
		Budget:      contracts.Budget{MaxModelCalls: 2},
	})
	if err != nil || loop.Metadata["stop_reason"] != "budget_exceeded" {
		t.Fatalf("loop=%#v err=%v", loop.Metadata, err)
	}

	handoff, err := rt.Handoff(context.Background(), contracts.HandoffInput{ToAgent: "math", Policy: contracts.PatternPolicy{AllowHandoff: true, AllowedAgents: []string{"math"}}})
	if err != nil || !handoff.Accepted {
		t.Fatalf("handoff=%#v err=%v", handoff, err)
	}

	needsHuman, err := rt.RunPattern(context.Background(), contracts.PatternRequest{
		PatternHint: "human_review",
		Policy:      contracts.PatternPolicy{RequireApproval: true},
	})
	if err != nil || !needsHuman.NeedsHuman || needsHuman.CheckpointID == "" {
		t.Fatalf("human=%#v err=%v", needsHuman, err)
	}
	resumed, err := rt.ResumePattern(context.Background(), needsHuman.CheckpointID, contracts.ResumeInput{Approved: true, Text: "continue"})
	if err != nil || resumed.Metadata["resumed_from"] != needsHuman.CheckpointID {
		t.Fatalf("resumed=%#v err=%v", resumed, err)
	}
	if !hasTrace(trace.Events, "checkpoint.saved", "human_review") {
		t.Fatalf("trace events = %#v", trace.Events)
	}
}

func hasTrace(events []contracts.TraceEvent, kind, pattern string) bool {
	for _, e := range events {
		if e.Kind == kind && e.Pattern == pattern {
			return true
		}
	}
	return false
}
