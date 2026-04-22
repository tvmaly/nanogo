package agent

import "context"

// SubagentSemaphore limits the number of concurrently running subagents.
type SubagentSemaphore struct {
	ch chan struct{}
}

// NewSubagentSemaphore creates a semaphore that allows at most n concurrent subagents.
func NewSubagentSemaphore(n int) *SubagentSemaphore {
	return &SubagentSemaphore{ch: make(chan struct{}, n)}
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (s *SubagentSemaphore) Acquire(ctx context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees a slot.
func (s *SubagentSemaphore) Release() {
	<-s.ch
}

// RunSubagent implements tools.Runner. It runs a subagent under the semaphore.
// The actual subagent execution is done by the provided run function.
func (s *SubagentSemaphore) RunSubagent(ctx context.Context, run func(context.Context) (string, error)) (string, error) {
	if err := s.Acquire(ctx); err != nil {
		return "", err
	}
	defer s.Release()
	return run(ctx)
}
