package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/heartbeat"
	"github.com/tvmaly/nanogo/modules/scheduler"
)

// config is the top-level configuration structure.
type config struct {
	LLM struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"llm"`
	Tools struct {
		Sources []toolSourceConfig `json:"sources"`
	} `json:"tools"`
	Transports []driverConfig `json:"transports"`
	Subagents  struct {
		MaxConcurrent int `json:"max_concurrent"`
		TimeoutS      int `json:"timeout_s"`
	} `json:"subagents"`
	Harness struct {
		Sensors []driverConfig `json:"sensors"`
		Guides  []driverConfig `json:"guides"`
	} `json:"harness"`
	Obs []struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"obs"`
	Scheduler struct {
		Driver string          `json:"driver"`
		Config json.RawMessage `json:"config"`
	} `json:"scheduler"`
	Heartbeats    []heartbeat.Heartbeat `json:"heartbeats"`
	Memory        json.RawMessage       `json:"memory"`
	Evolve        json.RawMessage       `json:"evolve"`
	AgentPatterns struct {
		Enabled        bool   `json:"enabled"`
		DefaultPattern string `json:"default_pattern"`
		RouterEnabled  bool   `json:"router_enabled"`
		TraceDir       string `json:"trace_dir"`
	} `json:"agent_patterns"`
}

type driverConfig struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
}

type toolSourceConfig struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
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
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("decode config %s: %w", path, err)
		}
		if err := cfg.validate(); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
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

func (c *config) validate() error {
	if c.LLM.Driver == "" {
		return fmt.Errorf("llm.driver: required")
	}
	for i, entry := range c.Tools.Sources {
		switch entry.Driver {
		case "", "builtin", "progressive":
		default:
			if _, err := tools.Build(entry.Driver, entry.Config); err != nil {
				return fmt.Errorf("tools.sources[%d].driver: %w", i, err)
			}
		}
	}
	for i, entry := range c.Obs {
		switch entry.Driver {
		case "slog", "file", "cost":
		default:
			return fmt.Errorf("obs[%d].driver: unknown driver %q", i, entry.Driver)
		}
	}
	if c.Scheduler.Driver != "" && c.Scheduler.Driver != "stdlib" {
		if _, err := scheduler.Build(c.Scheduler.Driver, c.Scheduler.Config); err != nil {
			return fmt.Errorf("scheduler.driver: %w", err)
		}
	}
	return nil
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
