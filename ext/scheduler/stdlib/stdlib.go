// Package stdlib provides an interval-based scheduler using time.Ticker.
// Specs like "every 30m", "every 1h", "every 10s" are supported.
package stdlib

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/scheduler"
)

func init() {
	scheduler.Register("stdlib", func(cfg json.RawMessage) (scheduler.Scheduler, error) {
		return New(), nil
	})
}

// Scheduler is an interval-based scheduler.
type Scheduler struct {
	mu      sync.Mutex
	jobs    map[string]*job
	started bool
}

type job struct {
	id       string
	spec     string
	interval time.Duration
	fn       func(context.Context)
	cancel   context.CancelFunc
}

// New returns a new stdlib Scheduler (not yet started).
func New() *Scheduler {
	return &Scheduler{jobs: map[string]*job{}}
}

// Schedule registers a new job. If the scheduler is already running, the job
// starts immediately. Spec must be of the form "every <n><unit>" where unit is
// s, m, or h.
func (s *Scheduler) Schedule(id, spec string, fn func(context.Context)) error {
	d, err := parseSpec(spec)
	if err != nil {
		return fmt.Errorf("stdlib scheduler: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		if j.cancel != nil {
			j.cancel()
		}
	}
	j := &job{id: id, spec: spec, interval: d, fn: fn}
	s.jobs[id] = j
	if s.started {
		s.startJob(j)
	}
	return nil
}

// Remove cancels and removes a job.
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("stdlib scheduler: job %q not found", id)
	}
	if j.cancel != nil {
		j.cancel()
	}
	delete(s.jobs, id)
	return nil
}

// List returns all registered jobs with approximate next fire time.
func (s *Scheduler) List() []scheduler.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]scheduler.Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		next := time.Now().Add(j.interval).Format(time.RFC3339)
		out = append(out, scheduler.Job{ID: j.id, Spec: j.spec, Next: next})
	}
	return out
}

// Start begins firing all registered jobs and fires newly registered jobs too.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	for _, j := range s.jobs {
		s.startJob(j)
	}
	// Stop all jobs when ctx is done.
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, j := range s.jobs {
			if j.cancel != nil {
				j.cancel()
			}
		}
	}()
	return nil
}

// startJob must be called with s.mu held.
func (s *Scheduler) startJob(j *job) {
	jCtx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	go func() {
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()
		for {
			select {
			case <-jCtx.Done():
				return
			case <-ticker.C:
				j.fn(jCtx)
			}
		}
	}()
}

// parseSpec parses "every 30m", "every 1h", "every 10s".
func parseSpec(spec string) (time.Duration, error) {
	spec = strings.TrimSpace(spec)
	if !strings.HasPrefix(spec, "every ") {
		return 0, fmt.Errorf("invalid spec %q: must start with 'every '", spec)
	}
	rest := strings.TrimPrefix(spec, "every ")
	rest = strings.TrimSpace(rest)
	if len(rest) < 2 {
		return 0, fmt.Errorf("invalid spec %q: too short", spec)
	}
	// Check two-char suffix "ms" before single-char.
	if strings.HasSuffix(rest, "ms") {
		numStr2 := rest[:len(rest)-2]
		n2, err2 := strconv.ParseInt(numStr2, 10, 64)
		if err2 != nil || n2 <= 0 {
			return 0, fmt.Errorf("invalid spec %q: bad number %q", spec, numStr2)
		}
		return time.Duration(n2) * time.Millisecond, nil
	}
	unitChar := rest[len(rest)-1]
	numStr := rest[:len(rest)-1]
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid spec %q: bad number %q", spec, numStr)
	}
	switch unitChar {
	case 's':
		return time.Duration(n) * time.Second, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid spec %q: unknown unit %q (want s, m, h, or ms)", spec, string(unitChar))
	}
}
