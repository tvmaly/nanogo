package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/domains/lessonfactory"
	"github.com/tvmaly/nanogo/ext/adaptive/reports"
	"github.com/tvmaly/nanogo/ext/agentpatterns"
	"github.com/tvmaly/nanogo/ext/meta"
	costobs "github.com/tvmaly/nanogo/ext/obs/cost"
	researchtool "github.com/tvmaly/nanogo/ext/tools/research"
	"github.com/tvmaly/nanogo/modules/memory"
	modulesession "github.com/tvmaly/nanogo/modules/session"
	"github.com/tvmaly/nanogo/modules/skills"
	"github.com/tvmaly/nanogo/modules/tools/builtin"
)

const version = "0.15.0"

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
		case "help":
			if err := runHelpCmd(flag.Args()[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
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
		case "adaptive":
			if err := runAdaptiveCmd(flag.Args()[1:], *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "lessonfactory":
			if err := runLessonFactoryCmd(flag.Args()[1:], *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "meta":
			if err := runMetaCmd(flag.Args()[1:], *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "voice":
			if err := runVoiceCmd(flag.Args()[1:], *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "browser":
			if err := runBrowserCmd(flag.Args()[1:], *configPath, *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "tui":
			if err := runTUICmd(flag.Args()[1:], *configPath, *skillsDir, *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "openaiapi":
			if err := runOpenAIAPICmd(flag.Args()[1:], *configPath, *skillsDir, *workspaceDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "gatewayws":
			if err := runGatewayWSCmd(flag.Args()[1:], *configPath, *skillsDir, *workspaceDir); err != nil {
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
	store := modulesession.NewStore(os.TempDir(), nil)
	cleanup, err := startObs(context.Background(), bus, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obs error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	memStore, _ := memory.NewStore(*workspaceDir)
	model := cfg.modelForSource("cli")

	if *prompt != "" {
		if err := runSingleShot(context.Background(), cfg, provider, store, bus, memStore, *prompt, model, "cli"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx := context.Background()
	stopHeartbeats, _ := startHeartbeats(ctx, cfg, provider, store, bus, memStore)
	defer stopHeartbeats()
	if len(cfg.Transports) > 0 {
		app := newTransportApp(transportAppConfig{
			Cfg:       cfg,
			Provider:  provider,
			Store:     store,
			Bus:       bus,
			MemStore:  memStore,
			Model:     model,
			SkillsDir: *skillsDir,
		})
		stopTransports, err := startConfiguredTransports(ctx, cfg, bus, app)
		if err != nil {
			fmt.Fprintf(os.Stderr, "transport error: %v\n", err)
			os.Exit(1)
		}
		defer stopTransports()
		select {}
	}
	// REPL mode
	if err := runREPL(ctx, cfg, provider, store, bus, model); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runLessonFactoryCmd(args []string, workspace string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo lessonfactory compile|review|approve|assign --lesson <id>")
	}
	f := lessonfactory.New(lessonfactory.Config{Root: workspace})
	switch args[0] {
	case "compile":
		fs := flag.NewFlagSet("lessonfactory compile", flag.ContinueOnError)
		source := fs.String("source", "", "rough lesson markdown path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *source == "" {
			return fmt.Errorf("--source is required")
		}
		b, err := f.ProcessInboxFile(context.Background(), *source)
		if err != nil {
			return err
		}
		fmt.Printf("lesson: %s\npath: %s\n", b.ID, filepath.Join(workspace, "lessons", "generated", b.ID))
		return nil
	case "research":
		fs := flag.NewFlagSet("lessonfactory research", flag.ContinueOnError)
		topic := fs.String("topic", "beginner yo-yo tricks", "research topic")
		childAge := fs.Int("child-age", 7, "child age")
		skillType := fs.String("skill-type", "physical", "physical|conceptual|mixed")
		driver := fs.String("driver", "fake", "fake|openrouter")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver == "openrouter" && os.Getenv("OPENROUTER_API_KEY") == "" {
			return fmt.Errorf("OPENROUTER_API_KEY is required for live research smoke")
		}
		src := researchtool.NewSource(researchtool.Config{Workspace: workspace, Enabled: true, Driver: *driver})
		out, err := src.Research(context.Background(), researchtool.Request{Topic: *topic, ChildAge: *childAge, SkillType: *skillType})
		if err != nil {
			return err
		}
		fmt.Printf("sources: %s\nguides: %d\nvideos: %d\n", out.SourcesPath, out.Guides, out.Videos)
		return nil
	case "review":
		fs := flag.NewFlagSet("lessonfactory review", flag.ContinueOnError)
		lesson := fs.String("lesson", "latest", "lesson id or latest")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		id, err := resolveLessonID(workspace, *lesson)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(workspace, "lessons", "generated", id, "review.md"))
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	case "approve":
		fs := flag.NewFlagSet("lessonfactory approve", flag.ContinueOnError)
		lesson := fs.String("lesson", "latest", "lesson id or latest")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		id, err := resolveLessonID(workspace, *lesson)
		if err != nil {
			return err
		}
		if err := f.RecordParentReview(context.Background(), id, lessonfactory.ParentReview{Approved: true, Notes: "approved from CLI"}); err != nil {
			return err
		}
		fmt.Printf("approved: %s\n", id)
		return nil
	case "assign":
		fs := flag.NewFlagSet("lessonfactory assign", flag.ContinueOnError)
		lesson := fs.String("lesson", "latest", "lesson id or latest")
		child := fs.String("child", "", "child id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *child == "" {
			return fmt.Errorf("--child is required")
		}
		id, err := resolveLessonID(workspace, *lesson)
		if err != nil {
			return err
		}
		if err := f.Assign(context.Background(), id, *child); err != nil {
			return err
		}
		fmt.Printf("assigned: %s to %s\n", id, *child)
		return nil
	default:
		return fmt.Errorf("unknown lessonfactory command %q", args[0])
	}
}

func resolveLessonID(workspace, id string) (string, error) {
	if id != "latest" {
		return id, nil
	}
	base := filepath.Join(workspace, "lessons", "generated")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no generated lessons found")
	}
	sort.Strings(names)
	return names[len(names)-1], nil
}

func runMetaCmd(args []string, workspace string) error {
	if len(args) < 2 || args[0] != "lesson" || args[1] != "create" {
		return fmt.Errorf("usage: nanogo meta lesson create --kind manim_lesson|browser_game_lesson --prompt <text> --runner fake")
	}
	fs := flag.NewFlagSet("meta lesson create", flag.ContinueOnError)
	kind := fs.String("kind", "", "lesson artifact kind")
	prompt := fs.String("prompt", "", "lesson prompt")
	runner := fs.String("runner", "fake", "artifact runner")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	svc := meta.NewService(workspace, meta.NewJSONLStore(workspace))
	res, err := svc.CreateLesson(context.Background(), meta.CreateLessonRequest{Kind: *kind, Prompt: *prompt, Runner: *runner})
	if err != nil {
		return err
	}
	fmt.Printf("lesson_id: %s\n", res.LessonID)
	fmt.Printf("run_id: %s\n", res.RunID)
	fmt.Printf("decision: %s\n", res.Decision)
	fmt.Printf("eligible_for_promotion: %t\n", res.Eligible)
	if res.VideoPath != "" {
		fmt.Printf("video_path: %s\n", res.VideoPath)
	}
	if res.PreviewPath != "" {
		fmt.Printf("preview_path: %s\n", res.PreviewPath)
	}
	if res.PreviewURL != "" {
		fmt.Printf("preview_url: %s\n", res.PreviewURL)
	}
	if res.ValidationPath != "" {
		fmt.Printf("validation_report: %s\n", res.ValidationPath)
	}
	fmt.Printf("bundle_path: %s\n", res.BundlePath)
	return nil
}

func runAdaptiveCmd(args []string, workspace string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo adaptive demo|inspect --child <id> --subject <subject> --topic <topic>")
	}
	fs := flag.NewFlagSet("adaptive "+args[0], flag.ContinueOnError)
	child := fs.String("child", "", "child id")
	subject := fs.String("subject", "", "subject")
	topic := fs.String("topic", "", "topic")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *child == "" || *subject == "" || *topic == "" {
		return fmt.Errorf("--child, --subject, and --topic are required")
	}
	ctx := context.Background()
	ar, err := archive.New(workspace)
	if err != nil {
		return err
	}
	switch args[0] {
	case "demo":
		d := adaptive.FakeDomain{}
		arts, err := d.Compile(ctx, adaptive.CompileRequest{ChildID: *child, Subject: *subject, Topic: *topic, SourceBody: "phase 12 adaptive demo"})
		if err != nil {
			return err
		}
		for i, art := range arts {
			if err := ar.AddArtifact(ctx, art); err != nil {
				return err
			}
			score := 0.35 + float64(i)*0.45
			out, err := d.Evaluate(ctx, art, adaptive.Attempt{
				ID: "demo-" + art.ID, ArtifactID: art.ID, ChildID: *child,
				StartedAt: time.Now().UTC(), Observations: map[string]any{"score": score},
			})
			if err != nil {
				return err
			}
			if err := ar.AddOutcome(ctx, out); err != nil {
				return err
			}
		}
		top, err := ar.Top(ctx, archive.Query{ChildID: *child, Subject: *subject, Topic: *topic, IncludeFailures: true}, 1)
		if err != nil {
			return err
		}
		summary, err := reports.WriteChildPatternSummary(ctx, workspace, ar, *child)
		if err != nil {
			return err
		}
		inspect, err := reports.Inspect(ctx, workspace, ar, reports.InspectQuery{ChildID: *child, Subject: *subject, Topic: *topic})
		if err != nil {
			return err
		}
		if len(top) > 0 {
			fmt.Printf("winner: %s\n", top[0].ID)
		}
		fmt.Printf("child patterns: %s\ninspect report: %s\n", summary, inspect)
		return nil
	case "inspect":
		path, err := reports.Inspect(ctx, workspace, ar, reports.InspectQuery{ChildID: *child, Subject: *subject, Topic: *topic})
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	default:
		return fmt.Errorf("unknown adaptive command %q", args[0])
	}
}

func runSingleShot(ctx context.Context, cfg *config, provider llm.Provider, store session.Store, bus event.Bus, memStore *memory.Store, prompt, model, source string) error {
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

	coord := builtin.NewAskUserCoordinator(bus, sess.ID())
	src, err := buildRuntimeToolSource(cfg, provider, store, bus, coord)
	if err != nil {
		return err
	}
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
func runREPL(ctx context.Context, cfg *config, provider llm.Provider, store session.Store, bus event.Bus, model string) error {
	sess, err := store.Create(newID())
	if err != nil {
		return err
	}
	coord := builtin.NewAskUserCoordinator(bus, sess.ID())
	src, err := buildRuntimeToolSource(cfg, provider, store, bus, coord)
	if err != nil {
		return err
	}

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
			coord = builtin.NewAskUserCoordinator(bus, sess.ID())
			src, err = buildRuntimeToolSource(cfg, provider, store, bus, coord)
			if err != nil {
				return err
			}
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
		cancel()
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

func toolNames(specs []contracts.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

func buildSkillDispatcher(loader *skills.Loader, runner *cliSkillRunner, cfg *config) skills.Dispatcher {
	if cfg != nil && cfg.AgentPatterns.Enabled {
		rt := agentpatterns.New(agentpatterns.Config{
			AgentRunner:    runner,
			DefaultPattern: cfg.AgentPatterns.DefaultPattern,
			RouterEnabled:  cfg.AgentPatterns.RouterEnabled,
		})
		return skills.NewDispatcherWithPatterns(loader, runner, rt)
	}
	return skills.NewDispatcher(loader, runner)
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
	store := modulesession.NewStore(os.TempDir(), nil)
	runner := &cliSkillRunner{provider: provider, store: store, bus: bus, model: cfg.modelForSource("cli"), cfg: cfg}
	d := buildSkillDispatcher(loader, runner, cfg)

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
	cfg      *config
}

var _ contracts.AgentRunner = (*cliSkillRunner)(nil)

func (r *cliSkillRunner) RunAgent(ctx context.Context, req contracts.AgentRequest) (contracts.AgentResult, error) {
	text, err := r.RunSkill(ctx, skills.RunSkillOpts{
		UserMsg: req.Prompt, SkillName: firstNonEmpty(req.Metadata["skill_name"], "pattern"),
		Model: firstNonEmpty(req.Metadata["model"], r.model), Tools: toolNames(req.Tools), Session: req.SessionID,
	})
	return contracts.AgentResult{Text: text, Metadata: req.Metadata}, err
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

	coord := builtin.NewAskUserCoordinator(r.bus, sess.ID())
	var src tools.Source
	runnerHolder := &sourceHolder{}
	spawnRunner := buildSubagentRunner(r.cfg, r.provider, runnerHolder, r.bus, r.store)
	if len(opts.Tools) > 0 {
		src = tools.NewFilteredSource(builtin.NewSource(r.bus, coord, spawnRunner), opts.Tools)
	} else {
		src = builtin.NewSource(r.bus, coord, spawnRunner)
	}
	runnerHolder.Set(src)

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
			if p, ok := evt.Payload.(builtin.AskUserPayload); ok {
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
