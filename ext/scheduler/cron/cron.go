// Package cron implements a scheduler using robfig/cron.
package cron

import (
	"context"
	"fmt"

	robfig "github.com/robfig/cron/v3"
)

// Scheduler wraps a robfig/cron instance for a single spec.
type Scheduler struct {
	spec string
	c    *robfig.Cron
}

// New validates spec and returns a Scheduler. Accepts both 5-field standard
// cron and 6-field (seconds as the first field).
func New(spec string) (*Scheduler, error) {
	if err := validate(spec); err != nil {
		return nil, err
	}
	return &Scheduler{spec: spec}, nil
}

// validate tries the 5-field parser first, then the 6-field parser.
func validate(spec string) error {
	p5 := robfig.NewParser(robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow | robfig.Descriptor)
	if _, err := p5.Parse(spec); err == nil {
		return nil
	}
	p6 := robfig.NewParser(robfig.Second | robfig.Minute | robfig.Hour | robfig.Dom | robfig.Month | robfig.Dow | robfig.Descriptor)
	if _, err := p6.Parse(spec); err == nil {
		return nil
	}
	return fmt.Errorf("invalid cron spec %q", spec)
}

// Start schedules fn to fire per the spec and begins the cron loop.
// Stops automatically when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context, fn func()) {
	s.c = robfig.New(robfig.WithSeconds())
	s.c.AddFunc(s.spec, fn) //nolint:errcheck — spec already validated in New
	s.c.Start()
	go func() {
		<-ctx.Done()
		s.c.Stop()
	}()
}

// Stop halts the cron loop immediately.
func (s *Scheduler) Stop() {
	if s.c != nil {
		s.c.Stop()
	}
}
