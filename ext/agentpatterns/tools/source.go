package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/contracts"
	coretools "github.com/tvmaly/nanogo/core/tools"
)

type Source struct {
	runtime contracts.PatternRuntime
}

func NewSource(runtime contracts.PatternRuntime) *Source {
	return &Source{runtime: runtime}
}

func (s *Source) Tools(context.Context, coretools.TurnInfo) ([]coretools.Tool, error) {
	return []coretools.Tool{
		&runTool{s.runtime},
		&statusTool{},
		&resumeTool{s.runtime},
		&listTool{},
	}, nil
}

type runTool struct{ runtime contracts.PatternRunner }

func (*runTool) Name() string { return "pattern_run" }
func (*runTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{"type": "function", "function": map[string]any{"name": "pattern_run", "description": "Run an agent pattern.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}}, "required": []string{"prompt"}}}})
}
func (t *runTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Prompt  string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("pattern_run: %w", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("pattern_run: prompt is required")
	}
	out, err := t.runtime.RunPattern(ctx, contracts.PatternRequest{Prompt: p.Prompt, PatternHint: p.Pattern})
	return out.Text, err
}

type resumeTool struct{ runtime contracts.PatternResumer }

func (*resumeTool) Name() string { return "pattern_resume" }
func (*resumeTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{"type": "function", "function": map[string]any{"name": "pattern_resume", "description": "Resume an agent pattern checkpoint.", "parameters": map[string]any{"type": "object", "properties": map[string]any{"checkpoint_id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}}, "required": []string{"checkpoint_id"}}}})
}
func (t *resumeTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		CheckpointID string `json:"checkpoint_id"`
		Text         string `json:"text"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("pattern_resume: %w", err)
	}
	if p.CheckpointID == "" {
		return "", fmt.Errorf("pattern_resume: checkpoint_id is required")
	}
	out, err := t.runtime.ResumePattern(ctx, p.CheckpointID, contracts.ResumeInput{Text: p.Text})
	return out.Text, err
}

type statusTool struct{}

func (*statusTool) Name() string { return "pattern_status" }
func (*statusTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{"type": "function", "function": map[string]any{"name": "pattern_status", "description": "Show pattern runtime status.", "parameters": map[string]any{"type": "object"}}})
}
func (*statusTool) Call(context.Context, json.RawMessage) (string, error) {
	return "pattern runtime ready", nil
}

type listTool struct{}

func (*listTool) Name() string { return "pattern_list" }
func (*listTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{"type": "function", "function": map[string]any{"name": "pattern_list", "description": "List available agent patterns.", "parameters": map[string]any{"type": "object"}}})
}
func (*listTool) Call(context.Context, json.RawMessage) (string, error) {
	return "single,router,supervisor_worker,sequential,parallel,loop,review,handoff,human_review", nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
