package agent

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
)

type SubagentSemaphore struct {
	ch chan struct{}
}

func NewSubagentSemaphore(n int) *SubagentSemaphore {
	return &SubagentSemaphore{ch: make(chan struct{}, n)}
}

func (s *SubagentSemaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SubagentSemaphore) Release() {
	<-s.ch
}

type SubagentRunnerConfig struct {
	Provider  llm.Provider
	Source    tools.Source
	Bus       event.Bus
	Semaphore *SubagentSemaphore
	Store     session.Store
}

type SubagentRunner struct {
	cfg     SubagentRunnerConfig
	counter atomic.Int64
}

var _ contracts.SubagentSpawner = (*SubagentRunner)(nil)

func NewSubagentRunner(cfg SubagentRunnerConfig) *SubagentRunner {
	if cfg.Semaphore == nil {
		cfg.Semaphore = NewSubagentSemaphore(4)
	}
	return &SubagentRunner{cfg: cfg}
}

func (r *SubagentRunner) SpawnSubagent(ctx context.Context, req contracts.SubagentRequest) (contracts.SubagentResult, error) {
	if req.Budget.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Budget.MaxDuration)
		defer cancel()
	}
	result, err := r.RunSubagent(ctx, tools.SubagentOpts{
		ParentSession: firstNonEmpty(req.ParentRunID, req.SessionID),
		Goal:          req.Prompt,
		Role:          req.Role,
		Model:         req.Metadata["model"],
		Tools:         req.AllowedTools,
	})
	if err != nil {
		return contracts.SubagentResult{}, err
	}
	return contracts.SubagentResult{Text: result}, nil
}

func (r *SubagentRunner) RunSubagent(ctx context.Context, opts tools.SubagentOpts) (string, error) {
	if err := r.cfg.Semaphore.Acquire(ctx); err != nil {
		return "", err
	}
	defer r.cfg.Semaphore.Release()

	ctx = context.WithValue(ctx, llm.CtxKeySubagent, true)

	store := r.cfg.Store
	if store == nil {
		return "", fmt.Errorf("subagent session store is required")
	}
	id := fmt.Sprintf("subagent-%s-%d", opts.ParentSession, r.counter.Add(1))
	sess, err := store.Create(id)
	if err != nil {
		return "", fmt.Errorf("subagent session: %w", err)
	}

	if opts.Role != "" {
		sess.Append(llm.Message{Role: "system", Content: "Role: " + opts.Role})
	}
	sess.Append(llm.Message{Role: "user", Content: opts.Goal})

	src := r.cfg.Source
	if len(opts.Tools) > 0 {
		src = tools.NewFilteredSource(src, opts.Tools)
	}

	loop := NewLoop(Config{
		Provider:   r.cfg.Provider,
		Source:     src,
		Session:    sess,
		Bus:        r.cfg.Bus,
		Model:      opts.Model,
		SourceName: "subagent",
		SubagentOf: opts.ParentSession,
	})

	if err := loop.Run(ctx); err != nil {
		return "", err
	}

	msgs := sess.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && msgs[i].Content != "" {
			return msgs[i].Content, nil
		}
	}
	return "", nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
