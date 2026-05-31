package obs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/modules/obs"
)

func TestObserverNormalizesRuntimeEvents(t *testing.T) {
	ctx := context.Background()
	store := obs.NewFakeStore()
	observer := obs.NewEventObserver(store, obs.ObserverConfig{
		Source: "core/event",
		Now:    func() time.Time { return time.Unix(20, 0).UTC() },
		NewID:  func(string) string { return "id-" + string(rune(len(string("x")))) },
	})

	events := []event.Event{
		{Kind: event.TurnStarted, Session: "s1", Turn: 1, At: time.Unix(10, 0).UTC()},
		{Kind: event.TokenDelta, Session: "s1", Payload: "hi"},
		{Kind: event.ToolCallStarted, Session: "s1", Payload: map[string]string{"tool": "read_file", "id": "tc1"}},
		{Kind: event.ToolCallResult, Session: "s1", Payload: map[string]string{"tool": "read_file", "result": "ok"}},
		{Kind: event.TurnCompleted, Session: "s1", Payload: event.TurnCompletedPayload{Model: "m1", InputTokens: 2, OutputTokens: 3}},
		{Kind: event.Error, Session: "s1", Payload: "boom"},
		{Kind: event.SkillTriggered, Session: "s1", Payload: "deploy"},
		{Kind: event.SensorSignal, Session: "s1", Payload: event.SignalPayload{SensorName: "vet", Severity: "error", Message: "fix it", Fix: "go vet"}},
	}
	for _, ev := range events {
		if err := observer.Observe(ctx, ev); err != nil {
			t.Fatalf("Observe %s: %v", ev.Kind, err)
		}
	}

	got, err := store.Query(ctx, obs.QuerySpec{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	wantTypes := []string{"run.start", "llm.token", "tool.start", "tool.result", "run.finish", "run.failure", "skill.triggered", "validation.signal"}
	if len(got.Records) != len(wantTypes) {
		t.Fatalf("records = %d, want %d", len(got.Records), len(wantTypes))
	}
	for i, typ := range wantTypes {
		if got.Records[i].Type != typ {
			t.Fatalf("record[%d].Type = %q, want %q", i, got.Records[i].Type, typ)
		}
		if got.Records[i].Attributes["event_kind"] != string(events[i].Kind) {
			t.Fatalf("event kind not preserved: %#v", got.Records[i].Attributes)
		}
	}
	if got.Records[4].Attributes["input_tokens"] != 2 {
		t.Fatalf("turn metadata = %#v", got.Records[4].Attributes)
	}
	if got.Records[5].Error == nil || got.Records[5].Error.Message != "boom" {
		t.Fatalf("error record = %#v", got.Records[5])
	}
}

func TestObserverDisabledAndFailurePolicy(t *testing.T) {
	ctx := context.Background()
	writer := failingWriter{err: errors.New("disk full")}

	disabled := obs.NewEventObserver(writer, obs.ObserverConfig{Disabled: true, FailurePolicy: obs.FailureFailFast})
	if err := disabled.Observe(ctx, event.Event{Kind: event.TurnStarted}); err != nil {
		t.Fatalf("disabled Observe: %v", err)
	}

	bestEffort := obs.NewEventObserver(writer, obs.ObserverConfig{FailurePolicy: obs.FailureBestEffort})
	if err := bestEffort.Observe(ctx, event.Event{Kind: event.TurnStarted}); err != nil {
		t.Fatalf("best effort err = %v", err)
	}

	failFast := obs.NewEventObserver(writer, obs.ObserverConfig{FailurePolicy: obs.FailureFailFast})
	if err := failFast.Observe(ctx, event.Event{Kind: event.TurnStarted}); err == nil {
		t.Fatal("expected fail-fast error")
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Append(context.Context, obs.ObservationRecord) error { return w.err }
