package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/browser"
)

func TestBrowserConfigAcceptedAndAddsToolsWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}},"browser":{"enabled":true,"driver":"fake","max_sessions":1}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	src, err := buildToolSourceFromConfig(cfg, event.NewBus(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := src.Tools(context.Background(), tools.TurnInfo{Session: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(ts, "browser_session_start") {
		t.Fatalf("missing browser_session_start")
	}
}

func TestBrowserConfigRejectsUnknownDriver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}},"browser":{"enabled":true,"driver":"mystery"}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected unknown browser driver error")
	}
}

func TestBuildBrowserServiceUsesAgentBrowserDriver(t *testing.T) {
	cfg := &config{}
	cfg.Browser.Enabled = true
	cfg.Browser.Driver = "agent-browser"
	svc, err := buildBrowserService(cfg, event.NewBus(), t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Policy().Driver != "agent-browser" {
		t.Fatalf("driver = %q", svc.Policy().Driver)
	}
}

func TestBrowserGatewayRuntimeRegistersOpsWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}},"browser":{"enabled":true,"driver":"fake"}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	rt, err := buildGatewayRuntime(path, "", t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer rt.cleanup()
	if !containsString(rt.service.Status().Methods, "browser.health") {
		t.Fatalf("browser health not registered: %v", rt.service.Status().Methods)
	}
}

func TestBrowserCommandDoctorWithFakeDriver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"llm":{"driver":"openai","config":{}}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runBrowserCmd([]string{"doctor", "--driver", "fake"}, path, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func hasTool(ts []tools.Tool, name string) bool {
	for _, t := range ts {
		if t.Name() == name {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestBrowserToolStartCanBeCalledFromConfigSource(t *testing.T) {
	cfg := &config{}
	cfg.Browser.Enabled = true
	cfg.Browser.Driver = "fake"
	src, err := buildToolSourceFromConfig(cfg, event.NewBus(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := src.Tools(context.Background(), tools.TurnInfo{Session: "s"})
	for _, tool := range ts {
		if tool.Name() != "browser_session_start" {
			continue
		}
		out, err := tool.Call(context.Background(), json.RawMessage(`{"session_name":"lesson"}`))
		if err != nil {
			t.Fatal(err)
		}
		var res browser.Session
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err)
		}
		if res.ID == "" {
			t.Fatalf("missing session id: %s", out)
		}
		return
	}
	t.Fatal("start tool not found")
}
