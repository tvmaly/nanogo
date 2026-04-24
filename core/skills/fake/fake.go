// Package fake provides a fake Dispatcher for testing.
package fake

import (
	"context"
	"fmt"

	"github.com/tvmaly/nanogo/core/skills"
)

// Dispatcher records Fire calls for inspection.
type Dispatcher struct {
	Calls []skills.Trigger
	Err   error
}

func (d *Dispatcher) Fire(_ context.Context, t skills.Trigger) error {
	d.Calls = append(d.Calls, t)
	return d.Err
}

// AgentRunner records RunSkill calls for inspection.
type AgentRunner struct {
	Calls  []skills.RunSkillOpts
	Result string
	Err    error
}

func (r *AgentRunner) RunSkill(_ context.Context, opts skills.RunSkillOpts) (string, error) {
	r.Calls = append(r.Calls, opts)
	if r.Err != nil {
		return "", r.Err
	}
	if r.Result != "" {
		return r.Result, nil
	}
	return fmt.Sprintf("ran skill %s", opts.SkillName), nil
}
