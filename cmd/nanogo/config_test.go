package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/tools"
	faketools "github.com/tvmaly/nanogo/core/tools/fake"
)

// TestLoadConfig_DefaultPath verifies that loadConfig("") uses ~/.nanogo/config.json
// when it exists, rather than synthesising from env vars.
func TestLoadConfig_DefaultPath(t *testing.T) {

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")
	raw := map[string]any{
		"llm": map[string]any{
			"driver": "openai",
			"config": map[string]any{
				"base_url":    "https://test.example.com/v1",
				"api_key_env": "TEST_KEY",
				"model":       "test-model",
			},
		},
	}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(cfgFile, b, 0644); err != nil {
		t.Fatal(err)
	}

	// Override the default config path for this test.
	orig := defaultConfigPath
	defaultConfigPath = cfgFile
	defer func() { defaultConfigPath = orig }()

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.LLM.Driver != "openai" {
		t.Errorf("driver = %q, want \"openai\"", cfg.LLM.Driver)
	}
	var llmCfg struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal(cfg.LLM.Config, &llmCfg); err != nil {
		t.Fatalf("unmarshal llm config: %v", err)
	}
	if llmCfg.BaseURL != "https://test.example.com/v1" {
		t.Errorf("base_url = %q, want test URL", llmCfg.BaseURL)
	}
}

// TestLoadConfig_EnvFallback verifies that when no default config file exists,
// loadConfig("") still returns a usable config from environment variables.
func TestLoadConfig_EnvFallback(t *testing.T) {

	orig := defaultConfigPath
	defaultConfigPath = "/nonexistent/path/config.json"
	defer func() { defaultConfigPath = orig }()

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig fallback: %v", err)
	}
	if cfg.LLM.Driver != "openai" {
		t.Errorf("driver = %q, want \"openai\"", cfg.LLM.Driver)
	}
}

func TestConfigLoadsToolSources(t *testing.T) {
	tools.Register("config_test", func(json.RawMessage) (tools.Source, error) {
		return faketools.NewSource(faketools.New("config_tool", "ok")), nil
	})
	cfg := &config{}
	cfg.Tools.Sources = []toolSourceConfig{{Driver: "config_test"}}
	src, err := buildToolSourceFromConfig(cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildToolSourceFromConfig: %v", err)
	}
	list, err := src.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if len(list) != 1 || list[0].Name() != "config_tool" {
		t.Fatalf("tools = %v", list)
	}

	cfg.Tools.Sources = []toolSourceConfig{{Driver: "does_not_exist"}}
	if _, err := buildToolSourceFromConfig(cfg, nil, nil, nil); err == nil {
		t.Fatal("expected unknown driver error")
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"llm":{"driver":"openai","config":{}},"surprise":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err = %v, want unknown field", err)
	}
}

func TestLoadConfigRejectsUnknownObsDriver(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}},"obs":[{"driver":"mystery"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "obs[0].driver") {
		t.Fatalf("err = %v, want obs[0].driver", err)
	}
}

func TestLoadConfigAcceptsDocumentedTopLevelSections(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{
		"llm":{"driver":"openai","config":{}},
		"subagents":{"max_concurrent":4,"timeout_s":600},
		"tools":{"sources":[{"driver":"builtin"}]},
		"transports":[{"driver":"cli"}],
		"scheduler":{"driver":"stdlib"},
		"heartbeats":[],
		"harness":{"sensors":[],"guides":[]},
		"obs":[],
		"memory":{"session_ttl_h":24},
		"evolve":{"enabled":false},
		"agent_patterns":{"enabled":true,"default_pattern":"single","router_enabled":true,"trace_dir":"/tmp/traces"}
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.LLM.Driver != "openai" || len(cfg.Transports) != 1 || cfg.Subagents.MaxConcurrent != 4 || !cfg.AgentPatterns.Enabled {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestAgentPatternsConfigAddsPatternTools(t *testing.T) {
	cfg := &config{}
	cfg.AgentPatterns.Enabled = true
	src, err := buildToolSourceFromConfig(cfg, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := src.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range list {
		if tool.Name() == "pattern_run" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("pattern_run not found in %d tools", len(list))
	}
}

func TestBuildProviderRejectsUnknownLLMDriver(t *testing.T) {
	t.Parallel()
	cfg := &config{}
	cfg.LLM.Driver = "missing"
	_, err := buildProvider(cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown llm driver") {
		t.Fatalf("err = %v, want unknown llm driver", err)
	}
}
