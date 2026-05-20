package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tvmaly/nanogo/core/event"
	coreruntime "github.com/tvmaly/nanogo/core/runtime"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/agentpatterns"
	patterntools "github.com/tvmaly/nanogo/ext/agentpatterns/tools"
	"github.com/tvmaly/nanogo/ext/tools/progressive"
	"github.com/tvmaly/nanogo/modules/tools/builtin"
)

type progressiveSourceConfig struct {
	Manifest string             `json:"manifest"`
	Sources  []toolSourceConfig `json:"sources"`
}

func buildToolSourceFromConfig(cfg *config, bus event.Bus, coord builtin.AskUserCoord, runner tools.Runner) (tools.Source, error) {
	var base tools.Source
	var err error
	if cfg == nil || len(cfg.Tools.Sources) == 0 {
		base = builtin.NewSource(bus, coord, runner)
	} else {
		base, err = buildToolSources(cfg.Tools.Sources, bus, coord, runner)
		if err != nil {
			return nil, err
		}
	}
	if cfg != nil && cfg.AgentPatterns.Enabled {
		rt := agentpatterns.New(agentpatterns.Config{
			DefaultPattern: cfg.AgentPatterns.DefaultPattern,
			RouterEnabled:  cfg.AgentPatterns.RouterEnabled,
			ToolRuntime:    tools.NewContractRuntime(base),
		})
		return coreruntime.NewMultiSource(base, patterntools.NewSource(rt)), nil
	}
	return base, nil
}

func buildToolSources(entries []toolSourceConfig, bus event.Bus, coord builtin.AskUserCoord, runner tools.Runner) (tools.Source, error) {
	sources := make([]tools.Source, 0, len(entries))
	for _, entry := range entries {
		src, err := buildToolSourceEntry(entry, bus, coord, runner)
		if err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	if len(sources) == 1 {
		return sources[0], nil
	}
	return coreruntime.NewMultiSource(sources...), nil
}

func buildToolSourceEntry(entry toolSourceConfig, bus event.Bus, coord builtin.AskUserCoord, runner tools.Runner) (tools.Source, error) {
	switch entry.Driver {
	case "", "builtin":
		return builtin.NewSource(bus, coord, runner), nil
	case "progressive":
		var pc progressiveSourceConfig
		if err := json.Unmarshal(entry.Config, &pc); err != nil {
			return nil, err
		}
		var child tools.Source
		var err error
		if len(pc.Sources) == 0 {
			child = builtin.NewSource(bus, coord, runner)
		} else {
			child, err = buildToolSources(pc.Sources, bus, coord, runner)
			if err != nil {
				return nil, err
			}
		}
		var manifest progressive.Manifest
		if pc.Manifest != "" {
			data, err := os.ReadFile(expandPath(pc.Manifest))
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, err
			}
		}
		return progressive.NewSource(child, manifest)
	default:
		return tools.Build(entry.Driver, entry.Config)
	}
}

func recordEvents(ctx context.Context, bus event.Bus, fn func(context.Context, event.Event) error) {
	sub := bus.Subscribe(ctx, event.TurnStarted, event.TokenDelta, event.ToolCallStarted, event.ToolCallResult,
		event.TurnCompleted, event.AskUser, event.MemoryUpdated, event.SkillTriggered, event.SensorSignal,
		event.HeartbeatFired, event.EvolveProposed, event.EvolveApplied, event.EvolveReverted, event.Error)
	for e := range sub {
		_ = fn(ctx, e)
	}
}
