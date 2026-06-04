package toolsource_test

import (
	"context"
	"encoding/json"
	"testing"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/browser/fake"
	"github.com/tvmaly/nanogo/modules/browser/toolsource"
)

func TestProgressiveBrowserVisibility(t *testing.T) {
	src, svc := newSource(t, browser.Policy{})
	names := toolNames(t, src)
	if !contains(names, "browser_session_start") {
		t.Fatalf("missing start tool: %v", names)
	}
	if contains(names, "browser_open") || contains(names, "browser_eval") {
		t.Fatalf("unexpected tools before session: %v", names)
	}
	if _, err := svc.Start(context.Background(), browser.StartRequest{SessionName: "lesson"}); err != nil {
		t.Fatal(err)
	}
	names = toolNames(t, src)
	for _, want := range []string{"browser_open", "browser_snapshot", "browser_action", "browser_text", "browser_screenshot", "browser_tabs", "browser_wait", "browser_close", "browser_media_seek"} {
		if !contains(names, want) {
			t.Fatalf("missing %s after session: %v", want, names)
		}
	}
	if contains(names, "browser_eval") {
		t.Fatalf("eval should be hidden by default: %v", names)
	}
}

func TestEvalVisibleOnlyWhenPolicyAllows(t *testing.T) {
	src, svc := newSource(t, browser.Policy{AllowEval: true})
	if _, err := svc.Start(context.Background(), browser.StartRequest{SessionName: "lesson"}); err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, src); !contains(names, "browser_eval") {
		t.Fatalf("expected eval when policy allows it: %v", names)
	}
}

func TestToolCallReturnsJSON(t *testing.T) {
	src, _ := newSource(t, browser.Policy{})
	var start coretools.Tool
	for _, tool := range toolsOf(t, src) {
		if tool.Name() == "browser_session_start" {
			start = tool
		}
	}
	if start == nil {
		t.Fatal("missing start tool")
	}
	out, err := start.Call(context.Background(), json.RawMessage(`{"session_name":"lesson","headed":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SessionID string `json:"session_id"`
		TabID     string `json:"tab_id"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("tool output is not json: %v", err)
	}
	if decoded.SessionID == "" || decoded.TabID == "" {
		t.Fatalf("missing ids: %s", out)
	}
}

func newSource(t *testing.T, p browser.Policy) (*toolsource.Source, *browser.Service) {
	t.Helper()
	svc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New(), Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	return toolsource.New(svc), svc
}

func toolNames(t *testing.T, src *toolsource.Source) []string {
	t.Helper()
	ts := toolsOf(t, src)
	out := make([]string, 0, len(ts))
	for _, tool := range ts {
		out = append(out, tool.Name())
	}
	return out
}

func toolsOf(t *testing.T, src *toolsource.Source) []coretools.Tool {
	t.Helper()
	ts, err := src.Tools(context.Background(), coretools.TurnInfo{Session: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
