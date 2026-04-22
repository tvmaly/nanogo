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
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/core/transport"
	clitransport "github.com/tvmaly/nanogo/ext/transport/cli"

	// Register the openai provider via init()
	_ "github.com/tvmaly/nanogo/ext/llm/openai"
)

const version = "0.2.0"

func main() {
	prompt := flag.String("p", "", "Prompt to send (single-shot mode)")
	configPath := flag.String("config", "", "Path to config JSON file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

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

	if *prompt != "" {
		// Single-shot mode: run one turn and exit
		if err := runSingleShot(context.Background(), provider, store, bus, *prompt); err != nil {
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

func runSingleShot(ctx context.Context, provider llm.Provider, store session.Store, bus event.Bus, prompt string) error {
	sess, err := store.Create("single-shot")
	if err != nil {
		return err
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
	return <-done
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

func loadConfig(path string) (*config, error) {
	if path == "" {
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func buildProvider(cfg *config) (llm.Provider, error) {
	return llm.Build(cfg.LLM.Driver, cfg.LLM.Config)
}

// ensure simpleApp satisfies transport.App (no longer using it but keep transport import alive)
var _ transport.App = (*replApp)(nil)

type replApp struct{}

func (*replApp) Submit(_ context.Context, _, _ string) error             { return nil }
func (*replApp) Resume(_ context.Context, _, _ string) error             { return nil }
func (*replApp) TriggerSkill(_ context.Context, _ string, _ map[string]any) error { return nil }

// Keep cli transport import used
var _ = clitransport.New
