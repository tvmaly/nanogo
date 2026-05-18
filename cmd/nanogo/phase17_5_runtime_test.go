package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/memory"
	"github.com/tvmaly/nanogo/modules/skills"
	"github.com/tvmaly/nanogo/modules/transport"
)

func TestRunSingleShotConfiguresSpawnRunner(t *testing.T) {
	t.Parallel()

	var chatCalls int
	provider := fakellm.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		chatCalls++
		ch := make(chan llm.Chunk, 3)
		if _, ok := ctx.Value(llm.CtxKeySubagent).(bool); ok {
			ch <- llm.Chunk{TextDelta: "subagent answer"}
			ch <- llm.Chunk{FinishReason: "stop"}
			close(ch)
			return ch, nil
		}
		if chatCalls == 1 {
			ch <- llm.Chunk{ToolCall: &llm.ToolCall{ID: "spawn-1", Name: "spawn", Args: json.RawMessage(`{"goal":"research it"}`)}}
			ch <- llm.Chunk{FinishReason: "tool_calls"}
			close(ch)
			return ch, nil
		}
		ch <- llm.Chunk{TextDelta: "parent answer"}
		ch <- llm.Chunk{FinishReason: "stop"}
		close(ch)
		return ch, nil
	})

	err := runSingleShot(context.Background(), &config{}, provider, session.NewStore(t.TempDir(), nil), event.NewBus(), nil, "use spawn", "model-a", "cli")
	if err != nil {
		t.Fatalf("runSingleShot: %v", err)
	}
	if chatCalls < 3 {
		t.Fatalf("chat calls = %d, want parent + subagent + final", chatCalls)
	}
}

func TestSkillRunnerConfiguresSpawnRunnerAndToolAllowlist(t *testing.T) {
	t.Parallel()

	var sawSubagent bool
	provider := fakellm.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		ch := make(chan llm.Chunk, 3)
		if _, ok := ctx.Value(llm.CtxKeySubagent).(bool); ok {
			sawSubagent = true
			ch <- llm.Chunk{TextDelta: "subagent skill answer"}
			ch <- llm.Chunk{FinishReason: "stop"}
			close(ch)
			return ch, nil
		}
		ch <- llm.Chunk{ToolCall: &llm.ToolCall{ID: "spawn-1", Name: "spawn", Args: json.RawMessage(`{"goal":"do delegated work","tools":["read_file"]}`)}}
		ch <- llm.Chunk{FinishReason: "tool_calls"}
		close(ch)
		return ch, nil
	})

	runner := &cliSkillRunner{provider: provider, store: session.NewStore(t.TempDir(), nil), bus: event.NewBus(), cfg: &config{}}
	_, err := runner.RunSkill(context.Background(), skills.RunSkillOpts{
		SkillName: "delegating-skill",
		UserMsg:   "delegate",
		Tools:     []string{"spawn", "read_file"},
	})
	if err != nil && !strings.Contains(err.Error(), "max tool iterations") {
		t.Fatalf("RunSkill: %v", err)
	}
	if !sawSubagent {
		t.Fatal("spawn did not invoke a configured subagent runner")
	}
}

func TestBuildSubagentRunnerUsesConfigLimits(t *testing.T) {
	t.Parallel()

	cfg := &config{}
	cfg.Subagents.MaxConcurrent = 1
	cfg.Subagents.TimeoutS = 1
	src := tools.Source(&emptySource{})
	runner := buildSubagentRunner(cfg, fakellm.New(), src, event.NewBus(), session.NewStore(t.TempDir(), nil))
	if runner == nil {
		t.Fatal("runner is nil")
	}
}

func TestTransportAppSubmitRunsAgentLoop(t *testing.T) {
	t.Parallel()

	provider := fakellm.New([]llm.Chunk{{TextDelta: "transport answer"}, {FinishReason: "stop"}})
	bus := event.NewBus()
	app := newTransportApp(transportAppConfig{
		Cfg:      &config{},
		Provider: provider,
		Store:    session.NewStore(t.TempDir(), nil),
		Bus:      bus,
		MemStore: mustMemoryStore(t),
		Model:    "model-a",
	})
	if err := app.Submit(context.Background(), "transport-session", "hello"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if provider.Calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.Calls)
	}
}

func TestStartConfiguredTransportsBuildsRegisteredDrivers(t *testing.T) {
	t.Parallel()

	cfg := &config{}
	cfg.Transports = []driverConfig{{Driver: "phase17_5_transport"}}
	var started atomic.Bool
	registerTestTransport("phase17_5_transport", &started)
	stop, err := startConfiguredTransports(context.Background(), cfg, event.NewBus(), nil)
	if err != nil {
		t.Fatalf("startConfiguredTransports: %v", err)
	}
	defer stop()
	deadline := time.After(time.Second)
	for !started.Load() {
		select {
		case <-deadline:
			t.Fatal("transport was not started")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func mustMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type emptySource struct{}

func (*emptySource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) { return nil, nil }

type testTransport struct {
	started *atomic.Bool
	done    chan struct{}
}

func registerTestTransport(name string, started *atomic.Bool) {
	transport.Register(name, func(_ json.RawMessage, _ event.Bus, _ transport.App) (transport.Transport, error) {
		return &testTransport{started: started, done: make(chan struct{})}, nil
	})
}

func (t *testTransport) Name() string { return "test" }

func (t *testTransport) Start(ctx context.Context, _ transport.App) error {
	t.started.Store(true)
	<-ctx.Done()
	close(t.done)
	return nil
}

func (t *testTransport) Stop(context.Context) error { return nil }
