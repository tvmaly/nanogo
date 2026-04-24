package event

import (
	"context"
	"sync"
	"time"
)

type Kind string

const (
	TurnStarted     Kind = "turn.started"
	TokenDelta      Kind = "turn.token"
	ToolCallStarted Kind = "tool.started"
	ToolCallResult  Kind = "tool.result"
	TurnCompleted   Kind = "turn.completed"
	AskUser         Kind = "ask.user"
	MemoryUpdated   Kind = "memory.updated"
	SkillTriggered  Kind = "skill.triggered"
	SensorSignal    Kind = "sensor.signal"
	HeartbeatFired  Kind = "heartbeat.fired"
	EvolveProposed  Kind = "evolve.proposed"
	EvolveApplied   Kind = "evolve.applied"
	EvolveReverted  Kind = "evolve.reverted"
	Error           Kind = "error"
)

type Event struct {
	Kind    Kind
	Session string
	Turn    int
	At      time.Time
	Payload any
}

type TurnCompletedPayload struct {
	Text, Model, Source, Skill, SubagentOf string
	InputTokens, OutputTokens, CachedInputTokens int
}

const bufSize = 256

type Bus interface {
	Publish(e Event)
	Subscribe(ctx context.Context, kinds ...Kind) <-chan Event
}

type sub struct {
	kinds map[Kind]struct{}
	ch    chan Event
	ctx   context.Context
}

type bus struct {
	mu   sync.Mutex
	subs []*sub
}

func NewBus() Bus { return &bus{} }

func (b *bus) Publish(e Event) {
	b.mu.Lock()
	var pruned []*sub
	for _, s := range b.subs {
		if s.ctx.Err() == nil {
			pruned = append(pruned, s)
		}
	}
	b.subs = pruned
	active := make([]*sub, len(pruned))
	copy(active, pruned)
	b.mu.Unlock()
	for _, s := range active {
		if _, ok := s.kinds[e.Kind]; !ok {
			continue
		}
		select {
		case s.ch <- e:
		default:
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
		}
	}
}

func (b *bus) Subscribe(ctx context.Context, kinds ...Kind) <-chan Event {
	km := make(map[Kind]struct{}, len(kinds))
	for _, k := range kinds {
		km[k] = struct{}{}
	}
	s := &sub{kinds: km, ch: make(chan Event, bufSize), ctx: ctx}
	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()
	go func() { <-ctx.Done(); close(s.ch) }()
	return s.ch
}
