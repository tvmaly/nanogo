// Package fake provides a test-double App for transport tests.
package fake

import (
	"context"
	"sync"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport"
)

// SubmitCall records one Submit invocation.
type SubmitCall struct{ Session, Message string }

// ResumeCall records one Resume invocation.
type ResumeCall struct{ Session, Answer string }

// TriggerCall records one TriggerSkill invocation.
type TriggerCall struct {
	Name string
	Args map[string]any
}

// App is a fake transport.App. It optionally publishes a TurnCompleted event
// on Submit so SSE handlers receive a terminal event during tests.
type App struct {
	mu       sync.Mutex
	Submits  []SubmitCall
	Resumes  []ResumeCall
	Triggers []TriggerCall
	Bus      event.Bus
}

var _ transport.App = (*App)(nil)

func (a *App) Submit(_ context.Context, session, message string) error {
	a.mu.Lock()
	a.Submits = append(a.Submits, SubmitCall{session, message})
	a.mu.Unlock()
	if a.Bus != nil {
		a.Bus.Publish(event.Event{
			Kind:    event.TurnCompleted,
			Session: session,
			Payload: event.TurnCompletedPayload{Text: "ok"},
		})
	}
	return nil
}

func (a *App) Resume(_ context.Context, session, answer string) error {
	a.mu.Lock()
	a.Resumes = append(a.Resumes, ResumeCall{session, answer})
	a.mu.Unlock()
	return nil
}

func (a *App) TriggerSkill(_ context.Context, name string, args map[string]any) error {
	a.mu.Lock()
	a.Triggers = append(a.Triggers, TriggerCall{name, args})
	a.mu.Unlock()
	return nil
}
