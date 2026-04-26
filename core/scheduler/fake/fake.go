// Package fake provides a test-double Scheduler.
package fake

import (
	"context"
	"sync"

	"github.com/tvmaly/nanogo/core/scheduler"
)

// Scheduler is a synchronous fake that records scheduled jobs and fires them
// manually via Fire.
type Scheduler struct {
	mu   sync.Mutex
	jobs map[string]entry
}

type entry struct {
	spec string
	fn   func(context.Context)
}

var _ scheduler.Scheduler = (*Scheduler)(nil)

func New() *Scheduler { return &Scheduler{jobs: map[string]entry{}} }

func (s *Scheduler) Schedule(id, spec string, fn func(context.Context)) error {
	s.mu.Lock()
	s.jobs[id] = entry{spec, fn}
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) List() []scheduler.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]scheduler.Job, 0, len(s.jobs))
	for id, e := range s.jobs {
		out = append(out, scheduler.Job{ID: id, Spec: e.spec})
	}
	return out
}

func (s *Scheduler) Start(_ context.Context) error { return nil }

// Fire manually invokes the job with the given id.
func (s *Scheduler) Fire(ctx context.Context, id string) bool {
	s.mu.Lock()
	e, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		return false
	}
	e.fn(ctx)
	return true
}
