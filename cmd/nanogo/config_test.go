package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
