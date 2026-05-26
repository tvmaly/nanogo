package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	helpfiles "github.com/tvmaly/nanogo/ext/help/files"
	openaiadapter "github.com/tvmaly/nanogo/ext/llm/openai"
	"github.com/tvmaly/nanogo/ext/transport/gatewayws"
	"github.com/tvmaly/nanogo/ext/transport/openaiapi"
	tuitransport "github.com/tvmaly/nanogo/ext/transport/tui"
	"github.com/tvmaly/nanogo/ext/voice/providers/xai"
	"github.com/tvmaly/nanogo/ext/voice/realtime"
	voicesession "github.com/tvmaly/nanogo/ext/voice/session"
	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/help"
	"github.com/tvmaly/nanogo/modules/tools/builtin"
)

type gatewayRuntime struct {
	cfg      *config
	provider llm.Provider
	store    session.Store
	bus      event.Bus
	service  *gateway.Service
	cleanup  func()
}

func buildGatewayRuntime(configPath, skillsDir, workspaceDir, source string) (*gatewayRuntime, error) {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	provider, err := buildProvider(cfg)
	if err != nil {
		return nil, err
	}
	bus := event.NewBus()
	store := session.NewStore(os.TempDir(), nil)
	cleanup, err := startObs(context.Background(), bus, cfg)
	if err != nil {
		return nil, err
	}
	_ = workspaceDir
	model := cfg.modelForSource(source)
	helpSvc, err := buildGatewayHelpService()
	if err != nil {
		cleanup()
		return nil, err
	}
	svc := gateway.New(gateway.Config{
		Provider:  provider,
		Store:     store,
		Bus:       bus,
		Model:     model,
		SkillsDir: skillsDir,
		SkillRunner: &cliSkillRunner{
			provider: provider,
			store:    store,
			bus:      bus,
			model:    model,
			cfg:      cfg,
		},
		SourceFactory: func(ctx context.Context, sessionID string) (tools.Source, error) {
			coord := builtin.NewAskUserCoordinator(bus, sessionID)
			return buildRuntimeToolSource(cfg, provider, store, bus, coord)
		},
		CostPath:      configuredCostPath(cfg),
		ModelCatalog:  buildModelCatalog(cfg),
		Voice:         buildTUIGatewayVoiceController(cfg),
		RealtimeVoice: buildXAIRealtimeController(workspaceDir),
		Help:          helpSvc,
	})
	return &gatewayRuntime{cfg: cfg, provider: provider, store: store, bus: bus, service: svc, cleanup: cleanup}, nil
}

func buildGatewayHelpService() (help.Service, error) {
	pack, err := helpfiles.New(defaultHelpRoot()).Load(context.Background())
	if err != nil {
		return nil, err
	}
	cat, err := help.NewCatalog(pack)
	if err != nil {
		return nil, err
	}
	return help.NewService(cat), nil
}

func buildModelCatalog(cfg *config) gateway.ModelCatalog {
	if cfg == nil || cfg.LLM.Driver != "openai" {
		return nil
	}
	var c openaiadapter.Config
	if err := json.Unmarshal(cfg.LLM.Config, &c); err != nil {
		return nil
	}
	if c.APIKey == "" && c.APIKeyEnv != "" {
		c.APIKey = os.Getenv(c.APIKeyEnv)
	}
	return openaiadapter.NewModelCatalog(c)
}

type tuiGatewayVoiceController struct {
	state     gateway.VoiceState
	available bool
}

func buildTUIGatewayVoiceController(cfg *config) gateway.VoiceController {
	if cfg == nil {
		return nil
	}
	available := cfg.Voice.Enabled || cfg.Voice.STT.DefaultProvider != "" || cfg.Voice.TTS.DefaultProvider != ""
	if !available {
		return nil
	}
	return &tuiGatewayVoiceController{available: true}
}

func (c *tuiGatewayVoiceController) State(context.Context, string) (gateway.VoiceState, error) {
	if !c.available {
		return gateway.VoiceState{}, fmt.Errorf("voice unavailable")
	}
	return c.state, nil
}

func (c *tuiGatewayVoiceController) Update(_ context.Context, sessionID string, patch gateway.VoicePatch) (gateway.VoiceState, error) {
	if !c.available {
		return gateway.VoiceState{}, fmt.Errorf("voice unavailable")
	}
	c.state.Session = sessionID
	if patch.STTEnabled != nil {
		c.state.STTEnabled = *patch.STTEnabled
	}
	if patch.TTSEnabled != nil {
		c.state.TTSEnabled = *patch.TTSEnabled
	}
	return c.state, nil
}

type xaiRealtimeController struct {
	workspace string
	manager   *voicesession.Manager
	sessionID string
	state     gateway.RealtimeVoiceState
}

func buildXAIRealtimeController(workspace string) gateway.RealtimeVoiceController {
	return &xaiRealtimeController{workspace: workspace}
}

func (c *xaiRealtimeController) Start(ctx context.Context) (gateway.RealtimeVoiceState, error) {
	if c.sessionID != "" {
		c.state.Connected = true
		return c.state, nil
	}
	cfg, err := xai.ConfigFromEnv()
	if err != nil {
		return gateway.RealtimeVoiceState{}, err
	}
	adapter := xai.New(cfg)
	c.manager = voicesession.NewManager(voicesession.Config{
		Workspace: c.workspace,
		Provider:  adapter,
		ProviderCfg: realtime.ProviderConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		},
		SessionUpdate: xai.SessionUpdate(cfg),
	})
	s, err := c.manager.Start(ctx)
	if err != nil {
		return gateway.RealtimeVoiceState{}, err
	}
	c.sessionID = s.ID
	c.state = gateway.RealtimeVoiceState{Provider: "xai", Model: cfg.Model, SessionID: s.ID, Connected: true}
	return c.state, nil
}

func (c *xaiRealtimeController) Stop(context.Context) (gateway.RealtimeVoiceState, error) {
	if c.manager != nil && c.sessionID != "" {
		if err := c.manager.Close(c.sessionID); err != nil {
			return c.state, err
		}
	}
	c.sessionID = ""
	c.state.Connected = false
	return c.state, nil
}

func (c *xaiRealtimeController) Status(context.Context) (gateway.RealtimeVoiceState, error) {
	return c.state, nil
}

func configuredCostPath(cfg *config) string {
	if cfg == nil {
		return defaultCostPath()
	}
	for _, entry := range cfg.Obs {
		if entry.Driver != "cost" {
			continue
		}
		var c struct {
			OutputPath string `json:"output_path"`
		}
		_ = json.Unmarshal(entry.Config, &c)
		if c.OutputPath != "" {
			return expandPath(c.OutputPath)
		}
	}
	return defaultCostPath()
}

func runTUICmd(args []string, configPath, skillsDir, workspaceDir string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	smoke := fs.Bool("smoke", false, "Run non-interactive TUI smoke and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rt, err := buildGatewayRuntime(configPath, skillsDir, workspaceDir, "tui")
	if err != nil {
		return err
	}
	defer rt.cleanup()
	if *smoke {
		return runTUISmoke(context.Background(), rt.service)
	}
	return tuitransport.Run(context.Background(), rt.service)
}

func runTUISmoke(ctx context.Context, svc *gateway.Service) error {
	models, err := svc.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("tui smoke models: %w", err)
	}
	fmt.Printf("tui smoke models=%d\n", len(models))
	resp, err := svc.SubmitChat(ctx, gateway.ChatRequest{Session: "tui-smoke", Message: "Reply with exactly: PHASE_19_8_OK"})
	if err != nil {
		return fmt.Errorf("tui smoke chat: %w", err)
	}
	fmt.Printf("tui smoke reply=%s\n", strings.TrimSpace(resp.Text))
	var costs gateway.CostSummary
	for i := 0; i < 10; i++ {
		costs, err = svc.CostSummary("tui-smoke")
		if err != nil {
			return fmt.Errorf("tui smoke cost: %w", err)
		}
		if costs.Turns > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("tui smoke cost turns=%d input=%d output=%d usd=%.6f\n", costs.Turns, costs.InputTokens, costs.OutputTokens, costs.CostUSD)
	if !strings.Contains(resp.Text, "PHASE_19_8_OK") {
		return fmt.Errorf("tui smoke reply missing PHASE_19_8_OK")
	}
	return nil
}

func runOpenAIAPICmd(args []string, configPath, skillsDir, workspaceDir string) error {
	serverCfg, err := parseOpenAIAPIConfig(args)
	if err != nil {
		return err
	}
	rt, err := buildGatewayRuntime(configPath, skillsDir, workspaceDir, "openaiapi")
	if err != nil {
		return err
	}
	defer rt.cleanup()
	s := openaiapi.New(serverCfg, rt.service)
	fmt.Fprintf(os.Stderr, "nanogo OpenAI-compatible API listening on %s\n", serverCfg.Addr)
	return s.Start()
}

func runGatewayWSCmd(args []string, configPath, skillsDir, workspaceDir string) error {
	serverCfg, err := parseGatewayWSConfig(args)
	if err != nil {
		return err
	}
	rt, err := buildGatewayRuntime(configPath, skillsDir, workspaceDir, "gatewayws")
	if err != nil {
		return err
	}
	defer rt.cleanup()
	s := gatewayws.New(serverCfg, rt.service)
	fmt.Fprintf(os.Stderr, "nanogo gateway WS listening on %s%s\n", serverCfg.Addr, serverCfg.Path)
	return s.Start()
}

func parseOpenAIAPIConfig(args []string) (openaiapi.Config, error) {
	fs := flag.NewFlagSet("openaiapi", flag.ContinueOnError)
	addr := fs.String("addr", ":8081", "HTTP listen address")
	bearer := fs.String("bearer", "", "Bearer token")
	bearerEnv := fs.String("bearer-env", "NANOGO_GATEWAY_TOKEN", "Bearer token environment variable")
	insecure := fs.Bool("insecure-loopback", false, "Allow no bearer token for local development")
	if err := fs.Parse(args); err != nil {
		return openaiapi.Config{}, err
	}
	return openaiapi.Config{Addr: *addr, Auth: openaiapi.AuthConfig{Bearer: *bearer, BearerEnv: *bearerEnv, InsecureAllowNoAuth: *insecure}}, nil
}

func parseGatewayWSConfig(args []string) (gatewayws.Config, error) {
	fs := flag.NewFlagSet("gatewayws", flag.ContinueOnError)
	addr := fs.String("addr", ":8082", "WebSocket listen address")
	path := fs.String("path", "/gateway", "WebSocket path")
	bearer := fs.String("bearer", "", "Bearer token")
	bearerEnv := fs.String("bearer-env", "NANOGO_GATEWAY_TOKEN", "Bearer token environment variable")
	insecure := fs.Bool("insecure-loopback", false, "Allow no bearer token for local development")
	if err := fs.Parse(args); err != nil {
		return gatewayws.Config{}, err
	}
	return gatewayws.Config{Addr: *addr, Path: *path, Auth: gatewayws.AuthConfig{Bearer: *bearer, BearerEnv: *bearerEnv, InsecureAllowNoAuth: *insecure}}, nil
}
