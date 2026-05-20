package contracts_test

import (
	"context"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
)

func TestFakesImplementContracts(t *testing.T) {
	var _ contracts.AgentRunner = (*contractfake.AgentRunner)(nil)
	var _ contracts.SubagentSpawner = (*contractfake.SubagentSpawner)(nil)
	var _ contracts.ToolCatalog = (*contractfake.ToolRuntime)(nil)
	var _ contracts.ToolInvoker = (*contractfake.ToolRuntime)(nil)
	var _ contracts.ToolRuntime = (*contractfake.ToolRuntime)(nil)
	var _ contracts.PatternRunner = (*contractfake.PatternRuntime)(nil)
	var _ contracts.PatternResumer = (*contractfake.PatternRuntime)(nil)
	var _ contracts.HandoffTarget = (*contractfake.PatternRuntime)(nil)
	var _ contracts.PatternRuntime = (*contractfake.PatternRuntime)(nil)
	var _ contracts.TraceSink = (*contractfake.TraceSink)(nil)
	var _ contracts.ApprovalGate = (*contractfake.ApprovalGate)(nil)
}

func TestTraceSinkRecordsEvents(t *testing.T) {
	sink := &contractfake.TraceSink{}
	event := contracts.TraceEvent{
		Version:   1,
		RunID:     "run-1",
		SessionID: "session-1",
		Pattern:   "single",
		Kind:      "step",
		CreatedAt: time.Unix(1, 0),
	}

	if err := sink.EmitTrace(context.Background(), event); err != nil {
		t.Fatalf("EmitTrace: %v", err)
	}
	if len(sink.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.Events))
	}
	if sink.Events[0].RunID != "run-1" {
		t.Fatalf("RunID = %q", sink.Events[0].RunID)
	}
}

func TestApprovalGateReturnsConfiguredDecision(t *testing.T) {
	gate := &contractfake.ApprovalGate{
		Result: contracts.ApprovalResult{Approved: true, Comment: "ok"},
	}

	got, err := gate.RequestApproval(context.Background(), contracts.ApprovalRequest{
		ID:      "approval-1",
		Reason:  "handoff",
		Summary: "try a specialist",
	})
	if err != nil {
		t.Fatalf("RequestApproval approve: %v", err)
	}
	if !got.Approved || got.Comment != "ok" {
		t.Fatalf("approval = %+v", got)
	}
	if len(gate.Requests) != 1 || gate.Requests[0].Reason != "handoff" {
		t.Fatalf("requests = %+v", gate.Requests)
	}

	gate.Result = contracts.ApprovalResult{Rejected: true, Comment: "no"}
	got, err = gate.RequestApproval(context.Background(), contracts.ApprovalRequest{ID: "approval-2"})
	if err != nil {
		t.Fatalf("RequestApproval reject: %v", err)
	}
	if !got.Rejected || got.Approved {
		t.Fatalf("rejection = %+v", got)
	}
}

func TestPhase18V3OrchestrationContractsAreUsable(t *testing.T) {
	runtime := &contractfake.PatternRuntime{
		RunResult: contracts.PatternResult{
			Text:         "started",
			CheckpointID: "checkpoint-1",
		},
		ResumeResult: contracts.PatternResult{Text: "resumed"},
		HandoffResult: contracts.HandoffResult{
			Accepted: true,
			Text:     "handed off",
		},
	}

	run, err := runtime.RunPattern(context.Background(), contracts.PatternRequest{
		ID:          "pattern-1",
		SessionID:   "session-1",
		StudentID:   "child-1",
		Prompt:      "help with magnets",
		PatternHint: "supervisor-worker",
		Policy:      contracts.PatternPolicy{AllowSubagents: true},
	})
	if err != nil {
		t.Fatalf("RunPattern: %v", err)
	}
	if run.CheckpointID != "checkpoint-1" {
		t.Fatalf("checkpoint = %q", run.CheckpointID)
	}

	resume, err := runtime.ResumePattern(context.Background(), "checkpoint-1", contracts.ResumeInput{Approved: true})
	if err != nil {
		t.Fatalf("ResumePattern: %v", err)
	}
	if resume.Text != "resumed" {
		t.Fatalf("resume text = %q", resume.Text)
	}

	handoff, err := runtime.Handoff(context.Background(), contracts.HandoffInput{
		ID:        "handoff-1",
		FromAgent: "voice",
		ToAgent:   "tutor",
		Reason:    "needs lesson runtime",
	})
	if err != nil {
		t.Fatalf("Handoff: %v", err)
	}
	if !handoff.Accepted {
		t.Fatalf("handoff not accepted: %+v", handoff)
	}
}
