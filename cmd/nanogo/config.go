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
	Voice   voiceConfig   `json:"voice"`
	Browser browserConfig `json:"browser"`
}

type driverConfig struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
}

type toolSourceConfig struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
}

type voiceConfig struct {
	Enabled       bool   `json:"enabled"`
	DefaultLocale string `json:"default_locale"`
	STT           struct {
		DefaultProvider string                         `json:"default_provider"`
		Providers       map[string]voiceProviderConfig `json:"providers"`
	} `json:"stt"`
	TTS struct {
		DefaultProvider string                         `json:"default_provider"`
		Providers       map[string]voiceProviderConfig `json:"providers"`
	} `json:"tts"`
	WebSocket struct {
		Enabled      bool            `json:"enabled"`
		Path         string          `json:"path"`
		InputFormat  json.RawMessage `json:"input_format"`
		OutputFormat json.RawMessage `json:"output_format"`
	} `json:"websocket"`
	Privacy struct {
		PersistAudio              bool `json:"persist_audio"`
		PersistPartialTranscripts bool `json:"persist_partial_transcripts"`
		RedactTranscriptsInEvents bool `json:"redact_transcripts_in_events"`
	} `json:"privacy"`
	WebSearch voiceWebSearchConfig `json:"web_search"`
}

type voiceProviderConfig struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config,omitempty"`
	Model  string          `json:"model,omitempty"`
	Voice  string          `json:"voice,omitempty"`
}

type voiceWebSearchConfig struct {
	Engine            string   `json:"engine,omitempty"`
	MaxResults        int      `json:"max_results,omitempty"`
	MaxTotalResults   int      `json:"max_total_results,omitempty"`
	SearchContextSize string   `json:"search_context_size,omitempty"`
	AllowedDomains    []string `json:"allowed_domains,omitempty"`
	ExcludedDomains   []string `json:"excluded_domains,omitempty"`
}

type browserConfig struct {
	Enabled                bool     `json:"enabled"`
	Driver                 string   `json:"driver,omitempty"`
	MaxSessions            int      `json:"max_sessions,omitempty"`
	SessionTTLSeconds      int      `json:"session_ttl_seconds,omitempty"`
	AllowedDomains         []string `json:"allowed_domains,omitempty"`
	AllowFileRoots         []string `json:"allow_file_roots,omitempty"`
	ArtifactRoot           string   `json:"artifact_root,omitempty"`
	AllowEval              bool     `json:"allow_eval,omitempty"`
	AllowUploads           bool     `json:"allow_uploads,omitempty"`
	AllowDownloads         bool     `json:"allow_downloads,omitempty"`
	AllowNonLoopbackCDP    bool     `json:"allow_non_loopback_cdp,omitempty"`
	IncludeEvalConsole     bool     `json:"include_eval_console,omitempty"`
	SnapshotMaxDepth       int      `json:"snapshot_max_depth,omitempty"`
	SnapshotMaxOutputBytes int      `json:"snapshot_max_output_bytes,omitempty"`
	RegistryPath           string   `json:"registry_path,omitempty"`
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
		case "slog", "file", "cost", "jsonl":
		default:
			return fmt.Errorf("obs[%d].driver: unknown driver %q", i, entry.Driver)
		}
	}
	if c.Scheduler.Driver != "" && c.Scheduler.Driver != "stdlib" {
		if _, err := scheduler.Build(c.Scheduler.Driver, c.Scheduler.Config); err != nil {
			return fmt.Errorf("scheduler.driver: %w", err)
		}
	}
	if err := c.validateVoice(); err != nil {
		return err
	}
	if err := c.validateBrowser(); err != nil {
		return err
	}
	return nil
}

func (c *config) validateBrowser() error {
	if !c.Browser.Enabled && c.Browser.Driver == "" {
		return nil
	}
	switch c.Browser.Driver {
	case "", "fake", "agent-browser":
	default:
		return fmt.Errorf("browser.driver: unknown driver %q", c.Browser.Driver)
	}
	if c.Browser.MaxSessions < 0 {
		return fmt.Errorf("browser.max_sessions: must be >= 0")
	}
	if c.Browser.SessionTTLSeconds < 0 {
		return fmt.Errorf("browser.session_ttl_seconds: must be >= 0")
	}
	return nil
}

func (c *config) validateVoice() error {
	if !c.Voice.Enabled && c.Voice.STT.DefaultProvider == "" && c.Voice.TTS.DefaultProvider == "" {
		return nil
	}
	if c.Voice.STT.DefaultProvider != "" {
		if _, ok := c.Voice.STT.Providers[c.Voice.STT.DefaultProvider]; !ok {
			return fmt.Errorf("voice.stt.default_provider: provider %q is not configured", c.Voice.STT.DefaultProvider)
		}
	}
	if c.Voice.TTS.DefaultProvider != "" {
		if _, ok := c.Voice.TTS.Providers[c.Voice.TTS.DefaultProvider]; !ok {
			return fmt.Errorf("voice.tts.default_provider: provider %q is not configured", c.Voice.TTS.DefaultProvider)
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
