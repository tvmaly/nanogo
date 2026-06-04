package toolsource

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/browser"
)

type Source struct {
	service *browser.Service
}

func New(service *browser.Service) *Source {
	return &Source{service: service}
}

func (s *Source) Tools(context.Context, coretools.TurnInfo) ([]coretools.Tool, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("browser tool source: service is required")
	}
	tools := []coretools.Tool{tool{name: "browser_session_start", schema: schema("browser_session_start"), call: s.start}}
	if s.service.HasSession() {
		tools = append(tools,
			tool{name: "browser_open", schema: schema("browser_open"), call: s.open},
			tool{name: "browser_snapshot", schema: schema("browser_snapshot"), call: s.snapshot},
			tool{name: "browser_action", schema: schema("browser_action"), call: s.action},
			tool{name: "browser_text", schema: schema("browser_text"), call: s.text},
			tool{name: "browser_screenshot", schema: schema("browser_screenshot"), call: s.screenshot},
			tool{name: "browser_tabs", schema: schema("browser_tabs"), call: s.tabs},
			tool{name: "browser_wait", schema: schema("browser_wait"), call: s.wait},
			tool{name: "browser_close", schema: schema("browser_close"), call: s.close},
			tool{name: "browser_media_seek", schema: schema("browser_media_seek"), call: s.mediaSeek},
		)
		if s.service.Policy().AllowEval {
			tools = append(tools, tool{name: "browser_eval", schema: schema("browser_eval"), call: s.eval})
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools, nil
}

type caller func(context.Context, json.RawMessage) (any, error)

type tool struct {
	name   string
	schema json.RawMessage
	call   caller
}

func (t tool) Name() string            { return t.name }
func (t tool) Schema() json.RawMessage { return t.schema }
func (t tool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	res, err := t.call(ctx, args)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Source) start(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.StartRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_session_start: %w", err)
	}
	return s.service.Start(ctx, req)
}

func (s *Source) open(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.NavigateRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_open: %w", err)
	}
	return s.service.Navigate(ctx, req)
}

func (s *Source) snapshot(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.SnapshotRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_snapshot: %w", err)
	}
	if !req.InteractiveOnly {
		req.InteractiveOnly = true
	}
	return s.service.Snapshot(ctx, req)
}

func (s *Source) action(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.ActionRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_action: %w", err)
	}
	return s.service.Act(ctx, req)
}

func (s *Source) text(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.TextRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_text: %w", err)
	}
	return s.service.Text(ctx, req)
}

func (s *Source) screenshot(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.ScreenshotRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_screenshot: %w", err)
	}
	return s.service.Screenshot(ctx, req)
}

func (s *Source) tabs(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.TabsRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_tabs: %w", err)
	}
	return s.service.Tabs(ctx, req)
}

func (s *Source) wait(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.WaitRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_wait: %w", err)
	}
	return s.service.Wait(ctx, req)
}

func (s *Source) close(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.CloseRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_close: %w", err)
	}
	if !req.CloseSession {
		req.CloseSession = true
	}
	return map[string]any{"closed": true}, s.service.Close(ctx, req)
}

func (s *Source) eval(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.EvalRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_eval: %w", err)
	}
	return s.service.Eval(ctx, req)
}

func (s *Source) mediaSeek(ctx context.Context, args json.RawMessage) (any, error) {
	var req browser.MediaSeekRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("browser_media_seek: %w", err)
	}
	return s.service.MediaSeek(ctx, req)
}

func schema(name string) json.RawMessage {
	props := map[string]any{}
	required := []string{}
	switch name {
	case "browser_session_start":
		props = map[string]any{"session_name": str(), "headed": map[string]any{"type": "boolean"}}
	case "browser_open":
		props = map[string]any{"session_id": str(), "url": str(), "wait_until": str(), "timeout_ms": integer()}
		required = []string{"session_id", "url"}
	case "browser_snapshot", "browser_text", "browser_screenshot", "browser_tabs", "browser_wait", "browser_close", "browser_media_seek", "browser_eval":
		props = map[string]any{"session_id": str()}
		required = []string{"session_id"}
	case "browser_action":
		props = map[string]any{"session_id": str(), "kind": str(), "target": map[string]any{"type": "object"}, "value": str()}
		required = []string{"session_id", "kind"}
	}
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": name,
			"parameters":  map[string]any{"type": "object", "properties": props, "required": required},
		},
	})
}

func str() map[string]any     { return map[string]any{"type": "string"} }
func integer() map[string]any { return map[string]any{"type": "integer"} }
func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
