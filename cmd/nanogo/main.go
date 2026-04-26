package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/memory"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/skills"
	"github.com/tvmaly/nanogo/core/tools"
	// Register extensions via init()
	_ "github.com/tvmaly/nanogo/ext/llm/openai"
	_ "github.com/tvmaly/nanogo/ext/llm/router"
	_ "github.com/tvmaly/nanogo/ext/scheduler/stdlib"
	_ "github.com/tvmaly/nanogo/ext/transport/cli"
	_ "github.com/tvmaly/nanogo/ext/transport/repl"
	_ "github.com/tvmaly/nanogo/ext/transport/rest"
)

const version = "0.7.0"

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

	memStore, _ := memory.NewStore(*workspaceDir)

	if *prompt != "" {
		if err := runSingleShot(context.Background(), provider, store, bus, memStore, *prompt); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// REPL mode
	if err := runREPL(context.Background(), provider, store, bus); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runSingleShot(ctx context.Context, provider llm.Provider, store session.Store, bus event.Bus, memStore *memory.Store, prompt string) error {
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
		Provider: provider,
		Source:   src,
		Session:  sess,
		Bus:      bus,
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
func runREPL(ctx context.Context, provider llm.Provider, store session.Store, bus event.Bus) error {
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
			Provider: provider,
			Source:   src,
			Session:  sess,
			Bus:      bus,
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
	runner := &cliSkillRunner{provider: provider, store: store, bus: bus}
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
		Provider: r.provider,
		Source:   src,
		Session:  sess,
		Bus:      r.bus,
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
