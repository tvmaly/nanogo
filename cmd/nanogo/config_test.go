package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
