package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/memory"
	"github.com/tvmaly/nanogo/modules/skills"
	"github.com/tvmaly/nanogo/modules/tools/builtin"
	"github.com/tvmaly/nanogo/modules/transport"
)

type sourceHolder struct {
	mu  sync.RWMutex
	src tools.Source
}

func (h *sourceHolder) Set(src tools.Source) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.src = src
}

func (h *sourceHolder) Tools(ctx context.Context, turn tools.TurnInfo) ([]tools.Tool, error) {
	h.mu.RLock()
	src := h.src
	h.mu.RUnlock()
	if src == nil {
		return nil, fmt.Errorf("tool source not configured")
	}
	return src.Tools(ctx, turn)
}

type timeoutRunner struct {
	timeout time.Duration
	inner   tools.Runner
}

func (r timeoutRunner) RunSubagent(ctx context.Context, opts tools.SubagentOpts) (string, error) {
	if r.timeout <= 0 {
		return r.inner.RunSubagent(ctx, opts)
	}
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.inner.RunSubagent(runCtx, opts)
}

func buildSubagentRunner(cfg *config, provider llm.Provider, src tools.Source, bus event.Bus, store session.Store) tools.Runner {
	if provider == nil || src == nil {
		return nil
	}
	maxConcurrent := 4
	timeout := time.Duration(0)
	if cfg != nil {
		if cfg.Subagents.MaxConcurrent > 0 {
			maxConcurrent = cfg.Subagents.MaxConcurrent
		}
		if cfg.Subagents.TimeoutS > 0 {
			timeout = time.Duration(cfg.Subagents.TimeoutS) * time.Second
		}
	}
	runner := agent.NewSubagentRunner(agent.SubagentRunnerConfig{
		Provider:  provider,
		Source:    src,
		Bus:       bus,
		Store:     store,
		Semaphore: agent.NewSubagentSemaphore(maxConcurrent),
	})
	if timeout > 0 {
		return timeoutRunner{timeout: timeout, inner: runner}
	}
	return runner
}

func buildRuntimeToolSource(cfg *config, provider llm.Provider, store session.Store, bus event.Bus, coord builtin.AskUserCoord) (tools.Source, error) {
	holder := &sourceHolder{}
	runner := buildSubagentRunner(cfg, provider, holder, bus, store)
	src, err := buildToolSourceFromConfig(cfg, bus, coord, runner)
	if err != nil {
		return nil, err
	}
	holder.Set(src)
	return src, nil
}

type transportAppConfig struct {
	Cfg       *config
	Provider  llm.Provider
	Store     session.Store
	Bus       event.Bus
	MemStore  *memory.Store
	Model     string
	SkillsDir string
}

type transportApp struct {
	cfg      transportAppConfig
	mu       sync.Mutex
	sessions map[string]session.Session
	coords   map[string]builtin.AskUserCoord
	pending  map[string]string
}

var _ transport.App = (*transportApp)(nil)

func newTransportApp(cfg transportAppConfig) *transportApp {
	app := &transportApp{
		cfg:      cfg,
		sessions: map[string]session.Session{},
		coords:   map[string]builtin.AskUserCoord{},
		pending:  map[string]string{},
	}
	if cfg.Bus != nil {
		go app.recordAskUserTurns()
	}
	return app
}

func (a *transportApp) recordAskUserTurns() {
	sub := a.cfg.Bus.Subscribe(context.Background(), event.AskUser)
	for e := range sub {
		p, ok := e.Payload.(builtin.AskUserPayload)
		if !ok || e.Session == "" || p.TurnID == "" {
			continue
		}
		a.mu.Lock()
		a.pending[e.Session] = p.TurnID
		a.mu.Unlock()
	}
}

func (a *transportApp) Submit(ctx context.Context, sessionID, message string) error {
	sess, coord, err := a.session(sessionID)
	if err != nil {
		return err
	}
	sess.Append(llm.Message{Role: "user", Content: message})
	src, err := buildRuntimeToolSource(a.cfg.Cfg, a.cfg.Provider, a.cfg.Store, a.cfg.Bus, coord)
	if err != nil {
		return err
	}
	loop := agent.NewLoop(agent.Config{
		Provider:   a.cfg.Provider,
		Source:     src,
		Session:    sess,
		Bus:        a.cfg.Bus,
		Model:      a.cfg.Model,
		SourceName: "transport",
	})
	return loop.Run(ctx)
}

func (a *transportApp) Resume(_ context.Context, sessionID, answer string) error {
	a.mu.Lock()
	coord := a.coords[sessionID]
	turnID := a.pending[sessionID]
	delete(a.pending, sessionID)
	a.mu.Unlock()
	if coord == nil {
		return fmt.Errorf("session %q has no pending ask_user coordinator", sessionID)
	}
	if turnID == "" {
		return fmt.Errorf("session %q has no pending ask_user turn", sessionID)
	}
	coord.Resume(turnID, answer)
	return nil
}

func (a *transportApp) TriggerSkill(ctx context.Context, name string, args map[string]any) error {
	if a.cfg.SkillsDir == "" {
		return fmt.Errorf("transport skill trigger requires a skills directory")
	}
	sks, err := skills.Discover(a.cfg.SkillsDir, nil)
	if err != nil {
		return fmt.Errorf("transport skill trigger: %w", err)
	}
	runner := &cliSkillRunner{provider: a.cfg.Provider, store: a.cfg.Store, bus: a.cfg.Bus, model: a.cfg.Model, cfg: a.cfg.Cfg}
	return skills.NewDispatcher(skills.NewLoader(sks), runner).Fire(ctx, skills.Trigger{
		Skill:  name,
		Source: skills.SourceCLI,
		Args:   args,
	})
}

func (a *transportApp) session(sessionID string) (session.Session, builtin.AskUserCoord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if sess := a.sessions[sessionID]; sess != nil {
		return sess, a.coords[sessionID], nil
	}
	sess, err := a.cfg.Store.Create(sessionID)
	if err != nil {
		return nil, nil, err
	}
	coord := builtin.NewAskUserCoordinator(a.cfg.Bus, sess.ID())
	a.sessions[sessionID] = sess
	a.coords[sessionID] = coord
	return sess, coord, nil
}

func startConfiguredTransports(ctx context.Context, cfg *config, bus event.Bus, app transport.App) (func(), error) {
	if cfg == nil || len(cfg.Transports) == 0 {
		return func() {}, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	var built []transport.Transport
	for _, entry := range cfg.Transports {
		t, err := transport.Build(entry.Driver, entry.Config, bus, app)
		if err != nil {
			cancel()
			return func() {}, err
		}
		built = append(built, t)
		go func(t transport.Transport) {
			_ = t.Start(ctx, app)
		}(t)
	}
	return func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		for _, t := range built {
			_ = t.Stop(stopCtx)
		}
	}, nil
}
