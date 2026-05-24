package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/gateway"
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

func TestTUICmdBuildsModelCatalogAndVoiceControls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"m1","name":"Model 1"}]}`))
	}))
	defer srv.Close()
	cfgPath := writeOpenAIGatewayTestConfig(t, srv.URL, true)
	rt, err := buildGatewayRuntime(cfgPath, t.TempDir(), t.TempDir(), "tui")
	if err != nil {
		t.Fatalf("buildGatewayRuntime: %v", err)
	}
	defer rt.cleanup()
	models, err := rt.service.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models = %#v", models)
	}
	if _, err := rt.service.Dispatch(context.Background(), gatewayRequest("voice.update", `{"session":"s1","stt_enabled":true}`)); err != nil {
		t.Fatalf("voice.update: %v", err)
	}
}

func TestTUISmokeUsesModelsChatAndCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"m1","name":"Model 1"}]}`))
	}))
	defer srv.Close()
	cfgPath := writeOpenAIGatewayTestConfig(t, srv.URL, false)
	rt, err := buildGatewayRuntime(cfgPath, t.TempDir(), t.TempDir(), "tui")
	if err != nil {
		t.Fatalf("buildGatewayRuntime: %v", err)
	}
	defer rt.cleanup()
	rt.service = gateway.New(gateway.Config{
		Provider:     fakellm.New([]llm.Chunk{{TextDelta: "PHASE_19_8_OK"}, {FinishReason: "stop", Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1}}}),
		Store:        rt.store,
		Bus:          rt.bus,
		Source:       testCommandSource{},
		Model:        "m1",
		ModelCatalog: buildModelCatalog(rt.cfg),
		CostPath:     configuredCostPath(rt.cfg),
	})
	if err := runTUISmoke(context.Background(), rt.service); err != nil {
		t.Fatalf("runTUISmoke: %v", err)
	}
}

type testCommandSource struct{}

func (testCommandSource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return nil, nil
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
	return writeGatewayTestConfigWithBaseURL(t, "", false)
}

func writeGatewayTestConfigWithBaseURL(t *testing.T, baseURL string, voiceEnabled bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if baseURL == "" {
		baseURL = "https://example.test/v1"
	}
	voice := ""
	if voiceEnabled {
		voice = `,"voice":{"enabled":true}`
	}
	raw := []byte(`{
		"llm":{"driver":"phase19_7_fake","config":{"model":"fake-model","base_url":"` + baseURL + `"}},
		"obs":[{"driver":"cost","config":{"output_path":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "cost.jsonl")) + `","prices":{}}}]
	` + voice + `}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeOpenAIGatewayTestConfig(t *testing.T, baseURL string, voiceEnabled bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	voice := ""
	if voiceEnabled {
		voice = `,"voice":{"enabled":true}`
	}
	raw := []byte(`{
		"llm":{"driver":"openai","config":{"model":"fake-model","base_url":"` + baseURL + `","api_key":"test"}},
		"obs":[{"driver":"cost","config":{"output_path":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "cost.jsonl")) + `","prices":{}}}]
	` + voice + `}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func gatewayRequest(method, params string) gateway.Request {
	return gateway.Request{Method: method, Params: json.RawMessage(params)}
}
