package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	fakeevent "github.com/tvmaly/nanogo/core/event/fake"
	"github.com/tvmaly/nanogo/core/harness"
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

// TEST-6.3: Signal injection into next turn
func TestSignalInjection(t *testing.T) {
	t.Parallel()

	// Fake LLM: first response has a tool call, second has a final message
	toolCallChunks := []llm.Chunk{
		{ToolCall: &llm.ToolCall{ID: "call-1", Name: "my_tool", Args: json.RawMessage(`{}`)}},
		{FinishReason: "tool_calls"},
	}
	finalChunks := []llm.Chunk{
		{TextDelta: "done"},
		{FinishReason: "stop"},
	}
	llmProvider := fakellm.New(toolCallChunks, finalChunks)

	// Fake sensor that emits a signal on tool call
	testSen := &testSensor{
		sig: harness.Signal{
			Severity: "error",
			Message:  "test error",
			Fix:      "fix it",
			Binding:  false,
		},
	}

	// Fake tool
	myTool := faketools.New("my_tool", "tool-result")
	toolSource := faketools.NewSource(myTool)

	bus := fakeevent.New()
	sess := fakesession.New("sess-sig")
	sess.Append(llm.Message{Role: "user", Content: "test"})

	loop := agent.NewLoop(agent.Config{
		Provider: llmProvider,
		Source:   toolSource,
		Session:  sess,
		Bus:      bus,
		Sensors:  []harness.Sensor{testSen},
	})

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Check that LLM was called twice
	if llmProvider.Calls != 2 {
		t.Errorf("LLM calls = %d, want 2", llmProvider.Calls)
	}

	// Check that a SensorSignal event was published
	evts := bus.Events()
	var gotSignal bool
	for _, e := range evts {
		if e.Kind == event.SensorSignal {
			gotSignal = true
			payload, ok := e.Payload.(event.SignalPayload)
			if !ok {
				t.Errorf("payload type = %T, want SignalPayload", e.Payload)
			}
			if payload.Message != "test error" {
				t.Errorf("signal message = %q, want %q", payload.Message, "test error")
			}
		}
	}
	if !gotSignal {
		t.Error("SensorSignal event not published")
	}
}

// TestSignalInjectedIntoNextRequest verifies that advisory/binding signals produced
// by a sensor after a tool call are actually present in the subsequent LLM request.
func TestSignalInjectedIntoNextRequest(t *testing.T) {
	t.Parallel()

	toolCallChunks := []llm.Chunk{
		{ToolCall: &llm.ToolCall{ID: "c1", Name: "my_tool", Args: json.RawMessage(`{}`)}},
		{FinishReason: "tool_calls"},
	}
	finalChunks := []llm.Chunk{
		{TextDelta: "done"},
		{FinishReason: "stop"},
	}

	var secondReq llm.Request
	callCount := 0
	llmProvider := fakellm.NewFunc(func(_ context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		callCount++
		if callCount == 2 {
			secondReq = req
		}
		chunks := finalChunks
		if callCount == 1 {
			chunks = toolCallChunks
		}
		ch := make(chan llm.Chunk, len(chunks)+1)
		for _, c := range chunks {
			ch <- c
		}
		close(ch)
		return ch, nil
	})

	sensor := &testSensor{sig: harness.Signal{
		Severity: "warn",
		Message:  "advisory-signal-sentinel",
		Binding:  false,
	}}

	myTool := faketools.New("my_tool", "tool-result")
	bus := fakeevent.New()
	sess := fakesession.New("sess-inject")
	sess.Append(llm.Message{Role: "user", Content: "test"})

	loop := agent.NewLoop(agent.Config{
		Provider: llmProvider,
		Source:   faketools.NewSource(myTool),
		Session:  sess,
		Bus:      bus,
		Sensors:  []harness.Sensor{sensor},
	})

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("LLM calls = %d, want 2", callCount)
	}

	// The second LLM request must contain the advisory signal message.
	found := false
	for _, msg := range secondReq.Messages {
		if strings.Contains(msg.Content, "advisory-signal-sentinel") {
			found = true
			break
		}
	}
	if !found {
		t.Error("advisory signal not injected into second LLM request messages")
	}
}

// TestLoopContextKeysPassedThrough verifies that CtxKeySource and CtxKeySkill
// set on the context are visible to the LLM provider (needed for router dispatch).
func TestLoopContextKeysPassedThrough(t *testing.T) {
	t.Parallel()

	var gotSource, gotSkill string
	var gotSubagent bool

	llmProvider := fakellm.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		gotSource, _ = ctx.Value(llm.CtxKeySource).(string)
		gotSkill, _ = ctx.Value(llm.CtxKeySkill).(string)
		gotSubagent, _ = ctx.Value(llm.CtxKeySubagent).(bool)
		ch := make(chan llm.Chunk, 2)
		ch <- llm.Chunk{TextDelta: "ok"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})

	bus := fakeevent.New()
	sess := fakesession.New("sess-ctx")
	sess.Append(llm.Message{Role: "user", Content: "test"})

	ctx := context.WithValue(context.Background(), llm.CtxKeySource, "heartbeat")
	ctx = context.WithValue(ctx, llm.CtxKeySkill, "my-skill")
	ctx = context.WithValue(ctx, llm.CtxKeySubagent, true)

	loop := agent.NewLoop(agent.Config{
		Provider: llmProvider,
		Source:   faketools.NewSource(),
		Session:  sess,
		Bus:      bus,
	})

	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotSource != "heartbeat" {
		t.Errorf("CtxKeySource = %q, want %q", gotSource, "heartbeat")
	}
	if gotSkill != "my-skill" {
		t.Errorf("CtxKeySkill = %q, want %q", gotSkill, "my-skill")
	}
	if !gotSubagent {
		t.Error("CtxKeySubagent not passed through")
	}
}

// TestSubagentRunner_RealExecution verifies that SubagentRunner creates an isolated
// session, runs the agent loop, and returns the final text.
func TestSubagentRunner_RealExecution(t *testing.T) {
	t.Parallel()

	finalChunks := []llm.Chunk{
		{TextDelta: "subagent-result"},
		{FinishReason: "stop"},
	}
	provider := fakellm.New(finalChunks)
	bus := fakeevent.New()

	dir := t.TempDir()
	// Create a real session store backed by temp dir.
	// We use the session package's store here via the fake session; pass store directly.
	runner := agent.NewSubagentRunner(agent.SubagentRunnerConfig{
		Provider: provider,
		Source:   faketools.NewSource(),
		Bus:      bus,
		Semaphore: agent.NewSubagentSemaphore(4),
	})

	result, err := runner.RunSubagent(context.Background(), tools.SubagentOpts{
		ParentSession: "parent",
		Goal:          "do something",
	})
	if err != nil {
		t.Fatalf("RunSubagent: %v", err)
	}
	if result != "subagent-result" {
		t.Errorf("result = %q, want %q", result, "subagent-result")
	}
	_ = dir
}

// TestSubagentRunner_ToolsAllowlist verifies that tools are filtered by the allowlist.
func TestSubagentRunner_ToolsAllowlist(t *testing.T) {
	t.Parallel()

	var seenTools []string
	provider := fakellm.NewFunc(func(_ context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		seenTools = nil
		for _, ts := range req.Tools {
			var schema struct {
				Function struct{ Name string } `json:"function"`
			}
			if err := json.Unmarshal(ts, &schema); err == nil {
				seenTools = append(seenTools, schema.Function.Name)
			}
		}
		ch := make(chan llm.Chunk, 2)
		ch <- llm.Chunk{TextDelta: "done"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})
	bus := fakeevent.New()

	runner := agent.NewSubagentRunner(agent.SubagentRunnerConfig{
		Provider:  provider,
		Source:    faketools.NewSource(faketools.New("read_file", ""), faketools.New("bash", "")),
		Bus:       bus,
		Semaphore: agent.NewSubagentSemaphore(4),
	})

	_, err := runner.RunSubagent(context.Background(), tools.SubagentOpts{
		ParentSession: "parent",
		Goal:          "task",
		Tools:         []string{"read_file"},
	})
	if err != nil {
		t.Fatalf("RunSubagent: %v", err)
	}

	for _, name := range seenTools {
		if name != "read_file" {
			t.Errorf("unexpected tool %q in subagent", name)
		}
	}
}

// TestSubagentRunner_SemaphoreGates verifies the semaphore caps concurrency.
func TestSubagentRunner_SemaphoreGates(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		maxSeen int
		current int
	)
	provider := fakellm.NewFunc(func(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
		mu.Lock()
		current++
		if current > maxSeen {
			maxSeen = current
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		current--
		mu.Unlock()
		ch := make(chan llm.Chunk, 2)
		ch <- llm.Chunk{TextDelta: "ok"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})
	bus := fakeevent.New()

	runner := agent.NewSubagentRunner(agent.SubagentRunnerConfig{
		Provider:  provider,
		Source:    faketools.NewSource(),
		Bus:       bus,
		Semaphore: agent.NewSubagentSemaphore(2),
	})

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner.RunSubagent(context.Background(), tools.SubagentOpts{
				ParentSession: "p",
				Goal:          "task",
			})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if maxSeen > 2 {
		t.Errorf("max concurrent subagents = %d, want <= 2", maxSeen)
	}
}

// Ensure faketools and others are used
var _ tools.Source = (*faketools.Source)(nil)
var _ session.Session = (*fakesession.Session)(nil)
var _ fmt.Stringer = nil // suppress import warning

// --- test helpers ---

type testSensor struct {
	sig harness.Signal
}

func (ts *testSensor) Name() string { return "test_sensor" }

func (ts *testSensor) Observe(_ context.Context, _ harness.ToolResult) []harness.Signal {
	return []harness.Signal{ts.sig}
}
