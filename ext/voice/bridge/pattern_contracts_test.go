package bridge

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
)

func TestVoiceBridgeTranscriptUsesPatternRunnerContract(t *testing.T) {
	patterns := &contractfake.PatternRuntime{RunResult: contracts.PatternResult{Text: "try common denominators"}}
	b := NewPatternBridge(patterns, nil)
	text, action, err := b.Handle(context.Background(), Intent{Type: "child_help_request", ChildID: "cross", Subject: "math", Topic: "fractions", Text: "I need help"})
	if err != nil {
		t.Fatal(err)
	}
	if action != nil || text != "try common denominators" || len(patterns.RunRequests) != 1 {
		t.Fatalf("text=%q action=%#v calls=%d", text, action, len(patterns.RunRequests))
	}
	req := patterns.RunRequests[0]
	if req.StudentID != "cross" || req.Context["subject"] != "math" || req.Context["topic"] != "fractions" {
		t.Fatalf("request = %#v", req)
	}
}

func TestVoiceBridgeDirectHandoffUsesContractTarget(t *testing.T) {
	patterns := &contractfake.PatternRuntime{HandoffResult: contracts.HandoffResult{Accepted: true, Text: "handoff accepted"}}
	b := NewPatternBridge(nil, patterns)
	text, action, err := b.Handle(context.Background(), Intent{Type: "handoff", ChildID: "cross", Topic: "lesson", Text: "handoff", Payload: map[string]any{"to_agent": "lesson_planner"}})
	if err != nil {
		t.Fatal(err)
	}
	if action != nil || text != "handoff accepted" || len(patterns.HandoffInputs) != 1 {
		t.Fatalf("text=%q action=%#v handoffs=%d", text, action, len(patterns.HandoffInputs))
	}
	if patterns.HandoffInputs[0].ToAgent != "lesson_planner" {
		t.Fatalf("handoff = %#v", patterns.HandoffInputs[0])
	}
}
