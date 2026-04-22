// Package fake provides a test-friendly Bus implementation.
package fake

import (
	"context"
	"sync"

	"github.com/tvmaly/nanogo/core/event"
)

// Bus records published events and delivers them to subscribers.
type Bus struct {
	mu       sync.Mutex
	events   []event.Event
	subs     []sub
}

type sub struct {
	kinds map[event.Kind]struct{}
	ch    chan event.Event
	ctx   context.Context
}

func New() *Bus { return &Bus{} }

func (b *Bus) Publish(e event.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	active := make([]sub, 0, len(b.subs))
	for _, s := range b.subs {
		if s.ctx.Err() != nil {
			continue
		}
		active = append(active, s)
	}
	b.subs = active
	b.mu.Unlock()

	for _, s := range active {
		if _, ok := s.kinds[e.Kind]; !ok {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

func (b *Bus) Subscribe(ctx context.Context, kinds ...event.Kind) <-chan event.Event {
	km := make(map[event.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		km[k] = struct{}{}
	}
	s := sub{kinds: km, ch: make(chan event.Event, 256), ctx: ctx}
	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()
	go func() { <-ctx.Done(); close(s.ch) }()
	return s.ch
}

// Events returns all published events.
func (b *Bus) Events() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]event.Event, len(b.events))
	copy(out, b.events)
	return out
}
