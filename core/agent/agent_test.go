package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	fakeevent "github.com/tvmaly/nanogo/core/event/fake"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	fakesession "github.com/tvmaly/nanogo/core/session/fake"
	"github.com/tvmaly/nanogo/core/tools"
	faketools "github.com/tvmaly/nanogo/core/tools/fake"
)

// --- TEST-2.7: Agent loop: tool-call iteration ---

func TestLoopToolCall(t *testing.T) {
	t.Parallel()

	// Fake LLM: first response has a tool call, second has a final message
	toolCallChunks := []llm.Chunk{
		{ToolCall: &llm.ToolCall{ID: "call-1", Name: "my_tool", Args: json.RawMessage(`{"x":1}`)}},
		{FinishReason: "tool_calls"},
	}
	finalChunks := []llm.Chunk{
		{TextDelta: "done"},
		{FinishReason: "stop"},
	}
	llmProvider := fakellm.New(toolCallChunks, finalChunks)

	// Fake tool that returns "tool-result"
	myTool := faketools.New("my_tool", "tool-result")
	toolSource := faketools.NewSource(myTool)

	bus := fakeevent.New()
	sess := fakesession.New("sess-loop")
	sess.Append(llm.Message{Role: "user", Content: "do something"})

	loop := agent.NewLoop(agent.Config{
		Provider: llmProvider,
		Source:   toolSource,
		Session:  sess,
		Bus:      bus,
	})

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Tool should have been called once
	if len(myTool.Calls) != 1 {
		t.Errorf("tool calls = %d, want 1", len(myTool.Calls))
	}

	// LLM should have been called twice (tool call turn + final turn)
	if llmProvider.Calls != 2 {
		t.Errorf("LLM calls = %d, want 2", llmProvider.Calls)
	}

	// Session should contain: user, assistant (tool call), tool result, assistant (final)
	msgs := sess.Messages()
	if len(msgs) < 3 {
		t.Errorf("session messages = %d, want >= 3", len(msgs))
	}

	// TurnCompleted event should be published
	evts := bus.Events()
	var gotCompleted bool
	for _, e := range evts {
		if e.Kind == event.TurnCompleted {
			gotCompleted = true
			payload, ok := e.Payload.(event.TurnCompletedPayload)
			if !ok {
				t.Errorf("TurnCompleted payload type = %T", e.Payload)
			}
			if payload.Text != "done" {
				t.Errorf("TurnCompleted.Text = %q, want %q", payload.Text, "done")
			}
		}
	}
	if !gotCompleted {
		t.Error("TurnCompleted event not published")
	}
}

// --- TEST-2.8: Agent loop: stop on error ---

func TestLoopError(t *testing.T) {
	t.Parallel()

	llmProvider := &errorLLM{err: errors.New("provider failure")}
	bus := fakeevent.New()
	sess := fakesession.New("sess-err")
	sess.Append(llm.Message{Role: "user", Content: "hello"})

	loop := agent.NewLoop(agent.Config{
		Provider: llmProvider,
		Source:   faketools.NewSource(),
		Session:  sess,
		Bus:      bus,
	})

	err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from Run")
	}

	// Error event should be published
	evts := bus.Events()
	var gotError bool
	for _, e := range evts {
		if e.Kind == event.Error {
			gotError = true
		}
	}
	if !gotError {
		t.Error("Error event not published")
	}
}

// --- TEST-2.15: Subagent concurrency cap enforced ---

func TestSubagentConcurrency(t *testing.T) {
	t.Parallel()

	const maxConcurrent = 4
	const totalSpawns = 8

	var (
		mu         sync.Mutex
		concurrent int32
		maxSeen    int32
	)

	// A runner that tracks concurrency
	runner := &trackingRunner{
		delay: 50 * time.Millisecond,
		onRun: func() {
			c := atomic.AddInt32(&concurrent, 1)
			mu.Lock()
			if c > maxSeen {
				maxSeen = c
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&concurrent, -1)
		},
	}

	sem := agent.NewSubagentSemaphore(maxConcurrent)
	var wg sync.WaitGroup
	for i := 0; i < totalSpawns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem.Acquire(context.Background())
			defer sem.Release()
			runner.run()
		}()
	}
	wg.Wait()

	if int(maxSeen) > maxConcurrent {
		t.Errorf("max concurrent = %d, want <= %d", maxSeen, maxConcurrent)
	}
}

// --- TEST-2.16: Subagent timeout ---

func TestSubagentTimeout(t *testing.T) {
	t.Parallel()

	// Run a subagent with a 100ms timeout that would otherwise hang
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	hangingLLM := &hangingProvider{}
	bus := fakeevent.New()
	sess := fakesession.New("sub-timeout")
	sess.Append(llm.Message{Role: "user", Content: "do it"})

	loop := agent.NewLoop(agent.Config{
		Provider: hangingLLM,
		Source:   faketools.NewSource(),
		Session:  sess,
		Bus:      bus,
	})

	err := loop.Run(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context error", err)
	}
}

// helpers

type errorLLM struct{ err error }

func (e *errorLLM) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	return nil, e.err
}

type hangingProvider struct{}

func (h *hangingProvider) Chat(ctx context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

type trackingRunner struct {
	delay time.Duration
	onRun func()
}

func (r *trackingRunner) run() {
	if r.onRun != nil {
		r.onRun()
	}
}

// Ensure faketools and others are used
var _ tools.Source = (*faketools.Source)(nil)
var _ session.Session = (*fakesession.Session)(nil)
var _ fmt.Stringer = nil // suppress import warning
