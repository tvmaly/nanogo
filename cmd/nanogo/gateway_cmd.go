package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/transport/gatewayws"
	"github.com/tvmaly/nanogo/ext/transport/openaiapi"
	tuitransport "github.com/tvmaly/nanogo/ext/transport/tui"
	"github.com/tvmaly/nanogo/modules/gateway"
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
		CostPath: configuredCostPath(cfg),
	})
	return &gatewayRuntime{cfg: cfg, provider: provider, store: store, bus: bus, service: svc, cleanup: cleanup}, nil
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	rt, err := buildGatewayRuntime(configPath, skillsDir, workspaceDir, "tui")
	if err != nil {
		return err
	}
	defer rt.cleanup()
	return tuitransport.Run(context.Background(), rt.service)
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
