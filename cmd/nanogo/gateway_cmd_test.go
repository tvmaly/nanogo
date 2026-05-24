package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
)

func init() {
	llm.Register("phase19_7_fake", func(json.RawMessage) (llm.Provider, error) {
		return fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}}), nil
	})
}

func TestTUICmdBuildsGatewayRuntime(t *testing.T) {
	cfgPath := writeGatewayTestConfig(t)
	skillsDir := t.TempDir()
	rt, err := buildGatewayRuntime(cfgPath, skillsDir, t.TempDir(), "tui")
	if err != nil {
		t.Fatalf("buildGatewayRuntime: %v", err)
	}
	defer rt.cleanup()
	if rt.service == nil || rt.provider == nil || rt.store == nil || rt.bus == nil {
		t.Fatalf("runtime = %#v", rt)
	}
	if rt.service.Status().Model != "fake-model" {
		t.Fatalf("model = %q", rt.service.Status().Model)
	}
	if _, err := rt.service.ToolCatalog(context.Background(), "s1"); err != nil {
		t.Fatalf("tool source factory not configured: %v", err)
	}
}

func TestOpenAIAPICmdParsesGatewayFlags(t *testing.T) {
	cfg, err := parseOpenAIAPIConfig([]string{"--addr", ":9991", "--bearer", "secret", "--bearer-env", "TOKEN_ENV", "--insecure-loopback"})
	if err != nil {
		t.Fatalf("parseOpenAIAPIConfig: %v", err)
	}
	if cfg.Addr != ":9991" || cfg.Auth.Bearer != "secret" || cfg.Auth.BearerEnv != "TOKEN_ENV" || !cfg.Auth.InsecureAllowNoAuth {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestGatewayWSCmdParsesGatewayFlags(t *testing.T) {
	cfg, err := parseGatewayWSConfig([]string{"--addr", ":9992", "--path", "/ws", "--bearer", "secret", "--bearer-env", "TOKEN_ENV", "--insecure-loopback"})
	if err != nil {
		t.Fatalf("parseGatewayWSConfig: %v", err)
	}
	if cfg.Addr != ":9992" || cfg.Path != "/ws" || cfg.Auth.Bearer != "secret" || cfg.Auth.BearerEnv != "TOKEN_ENV" || !cfg.Auth.InsecureAllowNoAuth {
		t.Fatalf("config = %#v", cfg)
	}
}

func writeGatewayTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	raw := []byte(`{
		"llm":{"driver":"phase19_7_fake","config":{"model":"fake-model"}},
		"obs":[{"driver":"cost","config":{"output_path":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "cost.jsonl")) + `","prices":{}}}]
	}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
