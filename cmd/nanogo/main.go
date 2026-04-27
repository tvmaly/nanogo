package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/heartbeat"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/memory"
	"github.com/tvmaly/nanogo/core/obs"
	"github.com/tvmaly/nanogo/core/scheduler"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/skills"
	"github.com/tvmaly/nanogo/core/tools"
	costobs "github.com/tvmaly/nanogo/ext/obs/cost"
	fileobs "github.com/tvmaly/nanogo/ext/obs/file"
	slogobs "github.com/tvmaly/nanogo/ext/obs/slog"
	// Register extensions via init()
	_ "github.com/tvmaly/nanogo/ext/llm/openai"
	_ "github.com/tvmaly/nanogo/ext/llm/router"
	_ "github.com/tvmaly/nanogo/ext/scheduler/stdlib"
	_ "github.com/tvmaly/nanogo/ext/transport/cli"
	_ "github.com/tvmaly/nanogo/ext/transport/repl"
	_ "github.com/tvmaly/nanogo/ext/transport/rest"
)

const version = "0.8.0"

func main() {
	prompt := flag.String("p", "", "Prompt to send (single-shot mode)")
	configPath := flag.String("config", "", "Path to config JSON file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	skillsDir := flag.String("skills", defaultSkillsDir(), "Directory containing skill .md files")
	workspaceDir := flag.String("workspace", defaultWorkspaceDir(), "Workspace directory for memory files")
	flag.Parse()

	// Handle subcommands before other flags.
	if flag.NArg() > 0 {
		switch flag.Arg(0) {
		case "cost":
			if err := runCostCmd(flag.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "skill":
			if err := runSkillCmd(flag.Args()[1:], *skillsDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "heartbeat":
			if err := runHeartbeatCmd(flag.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	provider, err := buildProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm error: %v\n", err)
		os.Exit(1)
	}

	bus := event.NewBus()
	store := session.NewStore(os.TempDir(), nil)
	cleanup, err := startObs(context.Background(), bus, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obs error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	memStore, _ := memory.NewStore(*workspaceDir)
	model := cfg.modelForSource("cli")

	if *prompt != "" {
		if err := runSingleShot(context.Background(), provider, store, bus, memStore, *prompt, model, "cli"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx := context.Background()
	stopHeartbeats, _ := startHeartbeats(ctx, cfg, provider, store, bus, memStore)
	defer stopHeartbeats()
	// REPL mode
	if err := runREPL(ctx, provider, store, bus, model); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runSingleShot(ctx context.Context, provider llm.Provider, store session.Store, bus event.Bus, memStore *memory.Store, prompt, model, source string) error {
	sess, err := store.Create("single-shot")
	if err != nil {
		return err
	}

	// Inject MEMORY.md as system context if present.
	if memStore != nil {
		if memContent, _ := memStore.ReadFile("MEMORY.md"); memContent != "" {
			sess.Append(llm.Message{Role: "system", Content: "## Long-term memory\n" + memContent})
		}
	}

	sess.Append(llm.Message{Role: "user", Content: prompt})

	// Subscribe to events to print output
	evtCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	tokens := bus.Subscribe(evtCtx, event.TokenDelta, event.TurnCompleted, event.Error)

	coord := tools.NewAskUserCoordinator(bus, sess.ID())
	src := tools.NewBuiltinSource(bus, coord, nil)
	loop := agent.NewLoop(agent.Config{
		Provider:   provider,
		Source:     src,
		Session:    sess,
		Bus:        bus,
		Model:      model,
		SourceName: source,
	})

	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	var text strings.Builder
	for evt := range tokens {
		switch evt.Kind {
		case event.TokenDelta:
			if s, ok := evt.Payload.(string); ok {
				text.WriteString(s)
				fmt.Print(s)
			}
		case event.TurnCompleted:
			cancel()
			fmt.Println()
		case event.Error:
			cancel()
			return fmt.Errorf("%v", evt.Payload)
		}
	}
	if err := <-done; err != nil {
		return err
	}

	// Persist conversation to history.jsonl and run a dream cycle.
	if memStore != nil {
		for _, msg := range sess.Messages() {
			if msg.Role == "system" {
				continue
			}
			_ = memStore.AppendHistory(memory.HistoryEntry{Role: msg.Role, Content: msg.Content})
		}
		dreamer := memory.NewDream(memStore, provider)
		_ = dreamer.Dream(ctx)
	}
	return nil
}

// runREPL starts an interactive read-eval-print loop.
func runREPL(ctx context.Context, provider llm.Provider, store session.Store, bus event.Bus, model string) error {
	sess, err := store.Create(newID())
	if err != nil {
		return err
	}
	coord := tools.NewAskUserCoordinator(bus, sess.ID())
	src := tools.NewBuiltinSource(bus, coord, nil)

	sc := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			fmt.Print("> ")
			continue
		}
		switch line {
		case "/exit", "/quit":
			return nil
		case "/new":
			sess, _ = store.Create(newID())
			coord = tools.NewAskUserCoordinator(bus, sess.ID())
			src = tools.NewBuiltinSource(bus, coord, nil)
			fmt.Println("[new session]")
			fmt.Print("> ")
			continue
		}

		sess.Append(llm.Message{Role: "user", Content: line})
		loop := agent.NewLoop(agent.Config{
			Provider:   provider,
			Source:     src,
			Session:    sess,
			Bus:        bus,
			Model:      model,
			SourceName: "cli",
		})

		evtCtx, cancel := context.WithCancel(ctx)
		tokens := bus.Subscribe(evtCtx, event.TokenDelta, event.TurnCompleted, event.Error)

		done := make(chan error, 1)
		go func() { done <- loop.Run(ctx) }()

		for evt := range tokens {
			switch evt.Kind {
			case event.TokenDelta:
				if s, ok := evt.Payload.(string); ok {
					fmt.Print(s)
				}
			case event.TurnCompleted, event.Error:
				cancel()
			}
		}
		<-done
		fmt.Println()
		fmt.Print("> ")
	}
	fmt.Println()
	return nil
}

var idCounter int

func newID() string {
	idCounter++
	return fmt.Sprintf("session-%d", idCounter)
}

// config is the top-level configuration structure.
type config struct {
	LLM struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"llm"`
	Obs []struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"obs"`
	Scheduler struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"scheduler"`
	Heartbeats []heartbeat.Heartbeat `json:"heartbeats"`
}

// defaultConfigPath is the path tried when no --config flag is given.
// Override in tests to avoid touching the real home directory.
var defaultConfigPath = func() string {
	home, _ := os.UserHomeDir()
	return home + "/.nanogo/config.json"
}()

func loadConfig(path string) (*config, error) {
	if path == "" {
		path = defaultConfigPath
	}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		var cfg config
		if err := json.NewDecoder(f).Decode(&cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	// File not found (or unreadable): synthesise from env vars.
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("NANOGO_MODEL")
	if model == "" {
		model = "anthropic/claude-haiku-4-5"
	}
	baseURL := os.Getenv("NANOGO_BASE_URL")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	raw, _ := json.Marshal(map[string]string{
		"base_url":    baseURL,
		"api_key_env": "OPENROUTER_API_KEY",
		"api_key":     apiKey,
		"model":       model,
	})
	cfg := &config{}
	cfg.LLM.Driver = "openai"
	cfg.LLM.Config = raw
	return cfg, nil
}

func buildProvider(cfg *config) (llm.Provider, error) {
	return llm.Build(cfg.LLM.Driver, cfg.LLM.Config)
}

func (c *config) modelForSource(source string) string {
	if c.LLM.Driver != "router" {
		var m struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(c.LLM.Config, &m)
		return m.Model
	}
	var rc struct {
		Providers map[string]struct {
			Config json.RawMessage `json:"config"`
		} `json:"providers"`
		Rules    []llm.Rule `json:"rules"`
		Fallback string     `json:"fallback"`
	}
	_ = json.Unmarshal(c.LLM.Config, &rc)
	route := rc.Fallback
	for _, r := range rc.Rules {
		if r.When == "source="+source || r.When == "default" {
			route = r.Route
			break
		}
	}
	var m struct {
		Model string `json:"model"`
	}
	if p, ok := rc.Providers[route]; ok {
		_ = json.Unmarshal(p.Config, &m)
	}
	return m.Model
}

func startObs(ctx context.Context, bus event.Bus, cfg *config) (func(), error) {
	var cancels []context.CancelFunc
	obs.Reset()
	for _, entry := range cfg.Obs {
		switch entry.Driver {
		case "slog":
			var c slogobs.Config
			_ = json.Unmarshal(entry.Config, &c)
			obs.SetLoggers(slogobs.New(c, os.Stderr))
		case "file":
			var c fileobs.Config
			if err := json.Unmarshal(entry.Config, &c); err != nil {
				return nil, err
			}
			c.Path = expandPath(c.Path)
			w, err := fileobs.New(c)
			if err != nil {
				return nil, err
			}
			subCtx, cancel := context.WithCancel(ctx)
			cancels = append(cancels, func() { cancel(); _ = w.Close() })
			go recordEvents(subCtx, bus, w.Record)
		case "cost":
			var c costobs.Config
			if err := json.Unmarshal(entry.Config, &c); err != nil {
				return nil, err
			}
			c.OutputPath = expandPath(c.OutputPath)
			t := costobs.New(c)
			subCtx, cancel := context.WithCancel(ctx)
			cancels = append(cancels, cancel)
			go recordEvents(subCtx, bus, t.Record)
		}
	}
	return func() {
		time.Sleep(100 * time.Millisecond)
		for _, c := range cancels {
			c()
		}
	}, nil
}

func recordEvents(ctx context.Context, bus event.Bus, fn func(context.Context, event.Event) error) {
	sub := bus.Subscribe(ctx, event.TurnStarted, event.TokenDelta, event.ToolCallStarted, event.ToolCallResult,
		event.TurnCompleted, event.AskUser, event.MemoryUpdated, event.SkillTriggered, event.SensorSignal,
		event.HeartbeatFired, event.EvolveProposed, event.EvolveApplied, event.EvolveReverted, event.Error)
	for e := range sub {
		_ = fn(ctx, e)
	}
}

func startHeartbeats(ctx context.Context, cfg *config, provider llm.Provider, store session.Store, bus event.Bus, memStore *memory.Store) (func(), error) {
	if len(cfg.Heartbeats) == 0 {
		return func() {}, nil
	}
	driver := cfg.Scheduler.Driver
	if driver == "" {
		driver = "stdlib"
	}
	sched, err := scheduler.Build(driver, cfg.Scheduler.Config)
	if err != nil {
		return func() {}, err
	}
	sub := heartbeatSubmitter{provider: provider, store: store, bus: bus, memStore: memStore, model: cfg.modelForSource("heartbeat")}
	rt := heartbeat.NewRuntime(sched, nil, tools.NewBuiltinSource(bus, nil, nil), sub, bus)
	for _, hb := range cfg.Heartbeats {
		_ = rt.Register(ctx, hb)
	}
	hbCtx, cancel := context.WithCancel(ctx)
	_ = sched.Start(hbCtx)
	return cancel, nil
}

type heartbeatSubmitter struct {
	provider llm.Provider
	store    session.Store
	bus      event.Bus
	memStore *memory.Store
	model    string
}

func (h heartbeatSubmitter) Submit(ctx context.Context, sessionID, message string) error {
	return runSingleShot(ctx, h.provider, h.store, h.bus, h.memStore, message, h.model, "heartbeat")
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func defaultCostPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nanogo", "cost.jsonl")
}

func runCostCmd(args []string) error {
	fs := flag.NewFlagSet("cost", flag.ContinueOnError)
	since := fs.String("since", "", "Filter duration: 24h, 7d, 30d")
	by := fs.String("by", "", "Group by model, skill, or source")
	path := fs.String("path", defaultCostPath(), "Cost JSONL path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := parseSince(*since)
	if err != nil {
		return err
	}
	out, err := costobs.Summary(expandPath(*path), *by, d)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func parseSince(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		n := strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(n, "%d", &days); err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func defaultSkillsDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.nanogo/skills"
}

func defaultWorkspaceDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.nanogo/workspace"
}

// runSkillCmd handles: skill list [--all] | skill run <name> [--key=val ...]
func runSkillCmd(args []string, skillsDir string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nanogo skill <list|run> [options]")
		return fmt.Errorf("missing subcommand")
	}
	switch args[0] {
	case "list":
		return skillList(args[1:], skillsDir)
	case "run":
		return skillRun(args[1:], skillsDir)
	default:
		return fmt.Errorf("unknown skill subcommand %q", args[0])
	}
}

func skillList(args []string, skillsDir string) error {
	fs := flag.NewFlagSet("skill list", flag.ContinueOnError)
	all := fs.Bool("all", false, "Include subagent skills")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sks, err := skills.Discover(skillsDir, nil)
	if err != nil {
		return fmt.Errorf("skill list: %w", err)
	}
	loader := skills.NewLoader(sks)

	list := loader.UserFacing()
	if *all {
		list = loader.All()
	}
	for _, sk := range list {
		label := ""
		if sk.Kind == "subagent" {
			label = " [subagent]"
		}
		fmt.Printf("%-30s %s%s\n", sk.Name, sk.Description, label)
	}
	return nil
}

func skillRun(args []string, skillsDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("skill run requires a skill name")
	}
	name := args[0]
	rest := args[1:]

	// Parse --key=val flags as skill args.
	skillArgs := map[string]any{}
	for _, a := range rest {
		a = strings.TrimPrefix(a, "--")
		parts := strings.SplitN(a, "=", 2)
		if len(parts) == 2 {
			skillArgs[parts[0]] = parts[1]
		}
	}

	sks, err := skills.Discover(skillsDir, nil)
	if err != nil {
		return fmt.Errorf("skill run: %w", err)
	}
	loader := skills.NewLoader(sks)

	cfg, err := loadConfig("")
	if err != nil {
		return err
	}
	provider, err := buildProvider(cfg)
	if err != nil {
		return err
	}

	bus := event.NewBus()
	store := session.NewStore(os.TempDir(), nil)
	runner := &cliSkillRunner{provider: provider, store: store, bus: bus, model: cfg.modelForSource("cli")}
	d := skills.NewDispatcher(loader, runner)

	return d.Fire(context.Background(), skills.Trigger{
		Skill:  name,
		Source: skills.SourceCLI,
		Args:   skillArgs,
	})
}

// cliSkillRunner implements skills.AgentRunner using the agent loop.
type cliSkillRunner struct {
	provider llm.Provider
	store    session.Store
	bus      event.Bus
	model    string
}

func (r *cliSkillRunner) RunSkill(ctx context.Context, opts skills.RunSkillOpts) (string, error) {
	// Set routing context keys so the LLM router can dispatch by skill name.
	ctx = context.WithValue(ctx, llm.CtxKeySkill, opts.SkillName)

	sess, err := r.store.Create("skill-" + opts.SkillName)
	if err != nil {
		return "", err
	}

	if opts.SystemNote != "" {
		sess.Append(llm.Message{Role: "system", Content: opts.SystemNote})
	}
	sess.Append(llm.Message{Role: "user", Content: opts.UserMsg})

	coord := tools.NewAskUserCoordinator(r.bus, sess.ID())
	var src tools.Source
	if len(opts.Tools) > 0 {
		src = tools.NewFilteredSource(tools.NewBuiltinSource(r.bus, coord, nil), opts.Tools)
	} else {
		src = tools.NewBuiltinSource(r.bus, coord, nil)
	}

	loop := agent.NewLoop(agent.Config{
		Provider:   r.provider,
		Source:     src,
		Session:    sess,
		Bus:        r.bus,
		Model:      firstNonEmpty(opts.Model, r.model),
		SourceName: "skill",
		SkillName:  opts.SkillName,
	})

	evtCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	tokens := r.bus.Subscribe(evtCtx, event.TokenDelta, event.TurnCompleted, event.Error, event.AskUser)

	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()

	sc := bufio.NewScanner(os.Stdin)
	var text strings.Builder
	for evt := range tokens {
		switch evt.Kind {
		case event.TokenDelta:
			if s, ok := evt.Payload.(string); ok {
				text.WriteString(s)
				fmt.Print(s)
			}
		case event.TurnCompleted:
			cancel()
			fmt.Println()
		case event.Error:
			cancel()
			return "", fmt.Errorf("%v", evt.Payload)
		case event.AskUser:
			if p, ok := evt.Payload.(tools.AskUserPayload); ok {
				fmt.Printf("\n%s\n> ", p.Question)
				if sc.Scan() {
					coord.Resume(p.TurnID, strings.TrimSpace(sc.Text()))
				} else {
					coord.Resume(p.TurnID, "")
				}
			}
		}
	}
	return text.String(), <-done
}
