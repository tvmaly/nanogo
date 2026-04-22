package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type spawnTool struct {
	session string
	runner  Runner
}

func newSpawnTool(session string, runner Runner) *spawnTool {
	return &spawnTool{session: session, runner: runner}
}

func (*spawnTool) Name() string { return "spawn" }
func (*spawnTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "spawn",
			"description": "Spawn a subagent to accomplish a goal concurrently. Returns the subagent's final message.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal":  map[string]any{"type": "string", "description": "What the subagent should accomplish."},
					"role":  map[string]any{"type": "string", "description": "Name of a subagent skill to use as system prompt."},
					"model": map[string]any{"type": "string", "description": "Model route key override."},
					"tools": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
						"description": "Allowlist of tool names the subagent may use.",
					},
				},
				"required": []string{"goal"},
			},
		},
	})
}

func (t *spawnTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("spawn: no runner configured")
	}
	var p struct {
		Goal  string   `json:"goal"`
		Role  string   `json:"role"`
		Model string   `json:"model"`
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("spawn: %w", err)
	}
	opts := SubagentOpts{
		ParentSession: t.session,
		Goal:          p.Goal,
		Role:          p.Role,
		Model:         p.Model,
		Tools:         p.Tools,
	}
	return t.runner.RunSubagent(ctx, opts)
}
