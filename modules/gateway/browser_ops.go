package gateway

import (
	"context"
	"encoding/json"

	"github.com/tvmaly/nanogo/modules/browser"
)

func (s *Service) registerBrowserOps() {
	if s.cfg.Browser == nil {
		return
	}
	register := func(method string, op func(context.Context, json.RawMessage) (any, error)) {
		s.registry.Register(method, op)
	}
	register("browser.health", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.cfg.Browser.Health(ctx)
	})
	register("browser.start", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.StartRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.start params")
		}
		return s.cfg.Browser.Start(ctx, req)
	})
	register("browser.navigate", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.NavigateRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.navigate params")
		}
		return s.cfg.Browser.Navigate(ctx, req)
	})
	register("browser.snapshot", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.SnapshotRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.snapshot params")
		}
		return s.cfg.Browser.Snapshot(ctx, req)
	})
	register("browser.action", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.ActionRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.action params")
		}
		return s.cfg.Browser.Act(ctx, req)
	})
	register("browser.text", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.TextRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.text params")
		}
		return s.cfg.Browser.Text(ctx, req)
	})
	register("browser.screenshot", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.ScreenshotRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.screenshot params")
		}
		return s.cfg.Browser.Screenshot(ctx, req)
	})
	register("browser.tabs", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.TabsRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.tabs params")
		}
		return s.cfg.Browser.Tabs(ctx, req)
	})
	register("browser.wait", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.WaitRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.wait params")
		}
		return s.cfg.Browser.Wait(ctx, req)
	})
	register("browser.close", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.CloseRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.close params")
		}
		return map[string]any{"closed": true}, s.cfg.Browser.Close(ctx, req)
	})
	register("browser.media.seek", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req browser.MediaSeekRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, "invalid browser.media.seek params")
		}
		return s.cfg.Browser.MediaSeek(ctx, req)
	})
}
