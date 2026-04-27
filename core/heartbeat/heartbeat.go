// Package heartbeat defines four action kinds and the runtime executing them.
package heartbeat

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/scheduler"
	"github.com/tvmaly/nanogo/core/skills"
	"github.com/tvmaly/nanogo/core/tools"
)

type ActionKind string

const (
	ActionWake    ActionKind = "wake"
	ActionSkill   ActionKind = "skill"
	ActionTrigger ActionKind = "trigger"
	ActionTool    ActionKind = "tool"
)

type Action struct {
	Kind   ActionKind     `json:"kind"`
	Prompt string         `json:"prompt,omitempty"`
	Skill  string         `json:"skill,omitempty"`
	When   string         `json:"when,omitempty"`
	Path   string         `json:"path,omitempty"`
	Tool   string         `json:"tool,omitempty"`
	Args   map[string]any `json:"args,omitempty"`
}

type Heartbeat struct {
	ID     string `json:"id"`
	Cron   string `json:"cron"`
	Action Action `json:"action"`
}

type Predicate interface {
	Name() string
	Evaluate(ctx context.Context) (bool, error)
}

type Submitter interface {
	Submit(ctx context.Context, session, message string) error
}

// ContextSourceKey is set to "heartbeat" in the context by the runtime.
type ContextSourceKey struct{}

var (
	predMu     sync.RWMutex
	predicates = map[string]Predicate{}
)

func RegisterPredicate(p Predicate) { predMu.Lock(); predicates[p.Name()] = p; predMu.Unlock() }
func LookupPredicate(name string) Predicate {
	predMu.RLock()
	defer predMu.RUnlock()
	return predicates[name]
}

type Runtime struct {
	sched      scheduler.Scheduler
	dispatcher skills.Dispatcher
	toolSource tools.Source
	submitter  Submitter
	bus        event.Bus
}

func NewRuntime(s scheduler.Scheduler, d skills.Dispatcher, ts tools.Source, sub Submitter, bus event.Bus) *Runtime {
	return &Runtime{sched: s, dispatcher: d, toolSource: ts, submitter: sub, bus: bus}
}

func (rt *Runtime) Register(ctx context.Context, hb Heartbeat) error {
	return rt.sched.Schedule(hb.ID, hb.Cron, func(ctx context.Context) { rt.execute(ctx, hb) })
}

func (rt *Runtime) Remove(id string) error { return rt.sched.Remove(id) }
func (rt *Runtime) List() []scheduler.Job  { return rt.sched.List() }

func (rt *Runtime) execute(ctx context.Context, hb Heartbeat) {
	rt.bus.Publish(event.Event{Kind: event.HeartbeatFired, Payload: hb})
	hbCtx := context.WithValue(ctx, ContextSourceKey{}, "heartbeat")
	hbCtx = context.WithValue(hbCtx, llm.CtxKeySource, "heartbeat")
	switch hb.Action.Kind {
	case ActionWake:
		if rt.submitter != nil {
			_ = rt.submitter.Submit(hbCtx, "heartbeat-"+hb.ID, hb.Action.Prompt)
		}
	case ActionSkill:
		if rt.dispatcher != nil {
			_ = rt.dispatcher.Fire(hbCtx, skills.Trigger{Skill: hb.Action.Skill, Source: skills.SourceHeartbeat})
			rt.bus.Publish(event.Event{Kind: event.SkillTriggered, Payload: hb.Action.Skill})
		}
	case ActionTrigger:
		p := LookupPredicate(hb.Action.When)
		if p == nil {
			return
		}
		if ok, err := p.Evaluate(hbCtx); err != nil || !ok {
			return
		}
		if rt.dispatcher != nil {
			_ = rt.dispatcher.Fire(hbCtx, skills.Trigger{Skill: hb.Action.Skill, Source: skills.SourceHeartbeat})
			rt.bus.Publish(event.Event{Kind: event.SkillTriggered, Payload: hb.Action.Skill})
		}
	case ActionTool:
		if rt.toolSource == nil {
			return
		}
		ts, err := rt.toolSource.Tools(hbCtx, tools.TurnInfo{Session: "heartbeat-" + hb.ID})
		if err != nil {
			return
		}
		argsJSON, _ := json.Marshal(hb.Action.Args)
		for _, t := range ts {
			if t.Name() == hb.Action.Tool {
				_, _ = t.Call(hbCtx, argsJSON)
				return
			}
		}
	}
}
