package heartbeat_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/heartbeat"
	fakehb "github.com/tvmaly/nanogo/core/heartbeat/fake"
	fakesched "github.com/tvmaly/nanogo/core/scheduler/fake"
	fakeskills "github.com/tvmaly/nanogo/core/skills/fake"
	"github.com/tvmaly/nanogo/core/tools"
)

func TestHeartbeatWake(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()
	sub := &fakehb.Submitter{}
	d := &fakeskills.Dispatcher{}
	rt := heartbeat.NewRuntime(sched, d, nil, sub, bus)

	hb := heartbeat.Heartbeat{
		ID:   "h1",
		Cron: "every 5m",
		Action: heartbeat.Action{
			Kind:   heartbeat.ActionWake,
			Prompt: "check inbox",
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "h1")

	got := <-evts
	if got.Kind != event.HeartbeatFired {
		t.Fatalf("expected HeartbeatFired, got %q", got.Kind)
	}
	if len(sub.Calls) != 1 {
		t.Fatalf("expected 1 Submit call, got %d", len(sub.Calls))
	}
	if sub.Calls[0].Message != "check inbox" {
		t.Fatalf("unexpected message %q", sub.Calls[0].Message)
	}
}

func TestHeartbeatSkill(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()
	d := &fakeskills.Dispatcher{}
	rt := heartbeat.NewRuntime(sched, d, nil, nil, bus)

	hb := heartbeat.Heartbeat{
		ID:   "h2",
		Cron: "every 1h",
		Action: heartbeat.Action{
			Kind:  heartbeat.ActionSkill,
			Skill: "daily-standup",
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.SkillTriggered, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "h2")

	got := <-evts
	if got.Kind != event.HeartbeatFired {
		t.Fatalf("expected HeartbeatFired first, got %q", got.Kind)
	}
	got = <-evts
	if got.Kind != event.SkillTriggered {
		t.Fatalf("expected SkillTriggered, got %q", got.Kind)
	}
	if len(d.Calls) != 1 || d.Calls[0].Skill != "daily-standup" {
		t.Fatalf("dispatcher not called with expected skill: %v", d.Calls)
	}
}

func TestHeartbeatTriggerTrue(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()
	d := &fakeskills.Dispatcher{}
	pred := &fakehb.Predicate{NameVal: "file_changed_true", ReturnVal: true}
	heartbeat.RegisterPredicate(pred)

	rt := heartbeat.NewRuntime(sched, d, nil, nil, bus)

	hb := heartbeat.Heartbeat{
		ID:   "h3",
		Cron: "every 1m",
		Action: heartbeat.Action{
			Kind:  heartbeat.ActionTrigger,
			When:  "file_changed_true",
			Skill: "process-inbox",
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.SkillTriggered, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "h3")

	<-evts // HeartbeatFired
	got := <-evts
	if got.Kind != event.SkillTriggered {
		t.Fatalf("expected SkillTriggered, got %q", got.Kind)
	}
	if len(d.Calls) != 1 || d.Calls[0].Skill != "process-inbox" {
		t.Fatalf("unexpected dispatcher calls: %v", d.Calls)
	}
}

func TestHeartbeatTriggerFalse(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()
	d := &fakeskills.Dispatcher{}
	pred := &fakehb.Predicate{NameVal: "never_true", ReturnVal: false}
	heartbeat.RegisterPredicate(pred)

	rt := heartbeat.NewRuntime(sched, d, nil, nil, bus)

	hb := heartbeat.Heartbeat{
		ID:   "h4",
		Cron: "every 1m",
		Action: heartbeat.Action{
			Kind:  heartbeat.ActionTrigger,
			When:  "never_true",
			Skill: "should-not-fire",
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "h4")

	<-evts // HeartbeatFired
	if len(d.Calls) != 0 {
		t.Fatalf("dispatcher should not be called when predicate returns false")
	}
}

func TestHeartbeatTool(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()

	ft := &fakeToolSource{}
	ft.tools = []tools.Tool{&fakeTool{name: "backup_memory", src: ft}}
	rt := heartbeat.NewRuntime(sched, nil, ft, nil, bus)

	hb := heartbeat.Heartbeat{
		ID:   "h5",
		Cron: "every 24h",
		Action: heartbeat.Action{
			Kind: heartbeat.ActionTool,
			Tool: "backup_memory",
			Args: map[string]any{"dst": "/tmp/backup"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "h5")

	<-evts
	if ft.callCount != 1 {
		t.Fatalf("expected 1 tool call, got %d", ft.callCount)
	}
}

func TestHeartbeatRouting(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	sched := fakesched.New()
	sub := &fakehb.Submitter{}
	rt := heartbeat.NewRuntime(sched, nil, nil, sub, bus)

	hb := heartbeat.Heartbeat{
		ID:   "route-test",
		Cron: "every 1m",
		Action: heartbeat.Action{
			Kind:   heartbeat.ActionWake,
			Prompt: "ping",
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := bus.Subscribe(ctx, event.HeartbeatFired)
	if err := rt.Register(ctx, hb); err != nil {
		t.Fatal(err)
	}
	sched.Fire(ctx, "route-test")
	<-evts

	if len(sub.Calls) != 1 {
		t.Fatalf("expected 1 submit, got %d", len(sub.Calls))
	}
}

// --- helpers ---

type fakeToolSource struct {
	tools     []tools.Tool
	callCount int
}

func (f *fakeToolSource) Tools(_ context.Context, _ tools.TurnInfo) ([]tools.Tool, error) {
	return f.tools, nil
}

type fakeTool struct {
	name string
	src  *fakeToolSource
}

func (t *fakeTool) Name() string             { return t.name }
func (t *fakeTool) Schema() json.RawMessage  { return json.RawMessage(`{}`) }
func (t *fakeTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	t.src.callCount++
	return "ok", nil
}
