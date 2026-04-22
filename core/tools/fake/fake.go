// Package fake provides a controllable Tool and Source for tests.
package fake

import (
	"context"
	"encoding/json"

	"github.com/tvmaly/nanogo/core/tools"
)

// Tool is a fake Tool that returns a fixed result.
type Tool struct {
	ToolName   string
	ToolResult string
	ToolErr    error
	Calls      []json.RawMessage
}

func New(name, result string) *Tool {
	return &Tool{ToolName: name, ToolResult: result}
}

func (t *Tool) Name() string { return t.ToolName }
func (t *Tool) Schema() json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.ToolName,
			"description": "fake tool",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	})
	return b
}
func (t *Tool) Call(_ context.Context, args json.RawMessage) (string, error) {
	t.Calls = append(t.Calls, args)
	return t.ToolResult, t.ToolErr
}

// Source is a fake Source that returns a fixed list of tools.
type Source struct {
	ToolList []tools.Tool
}

func NewSource(toolz ...tools.Tool) *Source {
	return &Source{ToolList: toolz}
}

func (s *Source) Tools(_ context.Context, _ tools.TurnInfo) ([]tools.Tool, error) {
	return s.ToolList, nil
}
