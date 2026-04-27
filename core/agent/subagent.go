package agent

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

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

func NewSubagentRunner(cfg SubagentRunnerConfig) *SubagentRunner {
	if cfg.Semaphore == nil {
		cfg.Semaphore = NewSubagentSemaphore(4)
	}
	return &SubagentRunner{cfg: cfg}
}

func (r *SubagentRunner) RunSubagent(ctx context.Context, opts tools.SubagentOpts) (string, error) {
	if err := r.cfg.Semaphore.Acquire(ctx); err != nil {
		return "", err
	}
	defer r.cfg.Semaphore.Release()

	ctx = context.WithValue(ctx, llm.CtxKeySubagent, true)

	store := r.cfg.Store
	if store == nil {
		store = session.NewStore(os.TempDir(), nil)
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
