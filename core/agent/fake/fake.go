// Package fake provides a controllable agent Runner for tests.
package fake

import (
	"context"

	"github.com/tvmaly/nanogo/core/tools"
)

// Runner is a fake tools.Runner that returns a fixed result.
type Runner struct {
	Result   string
	Err      error
	LastOpts tools.SubagentOpts
}

func New(result string) *Runner {
	return &Runner{Result: result}
}

func (r *Runner) RunSubagent(ctx context.Context, opts tools.SubagentOpts) (string, error) {
	r.LastOpts = opts
	return r.Result, r.Err
}
