// Package fake provides test doubles for heartbeat.
package fake

import (
	"context"
	"sync"

	"github.com/tvmaly/nanogo/core/heartbeat"
)

// Submitter records Submit calls.
type Submitter struct {
	mu   sync.Mutex
	Calls []struct{ Session, Message string }
}

var _ heartbeat.Submitter = (*Submitter)(nil)

func (s *Submitter) Submit(_ context.Context, session, message string) error {
	s.mu.Lock()
	s.Calls = append(s.Calls, struct{ Session, Message string }{session, message})
	s.mu.Unlock()
	return nil
}

// Predicate is a controllable Predicate for tests.
type Predicate struct {
	NameVal   string
	ReturnVal bool
}

var _ heartbeat.Predicate = (*Predicate)(nil)

func (p *Predicate) Name() string { return p.NameVal }
func (p *Predicate) Evaluate(_ context.Context) (bool, error) {
	return p.ReturnVal, nil
}
