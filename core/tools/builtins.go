package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

// NewBuiltinSource returns a Source containing all five core builtin tools.
// coord may be nil (ask_user will return an error if called without one).
// runner may be nil (spawn will return an error if called without one).
func NewBuiltinSource(bus event.Bus, coord AskUserCoord, runner Runner) Source {
	return &builtinSource{bus: bus, coord: coord, runner: runner}
}

type builtinSource struct {
	bus     event.Bus
	coord   AskUserCoord
	runner  Runner
}

func (s *builtinSource) Tools(ctx context.Context, turn TurnInfo) ([]Tool, error) {
	return []Tool{
		&readFileTool{},
		&writeFileTool{},
		&bashTool{},
		newAskUserTool(s.coord),
		newSpawnTool(turn.Session, s.runner),
	}, nil
}

// --- read_file ---

type readFileTool struct{}

func (*readFileTool) Name() string { return "read_file" }
func (*readFileTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "Read the contents of a file from disk.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Path of the file to read."},
					"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
					"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to return."},
				},
				"required": []string{"path"},
			},
		},
	})
}

func (*readFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read_file %s: %w", p.Path, err)
	}
	return string(data), nil
}

// --- write_file ---

type writeFileTool struct{}

func (*writeFileTool) Name() string { return "write_file" }
func (*writeFileTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "write_file",
			"description": "Write content to a file, creating parent directories as needed.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Destination file path."},
					"content": map[string]any{"type": "string", "description": "File content to write."},
				},
				"required": []string{"path", "content"},
			},
		},
	})
}

func (*writeFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0755); err != nil {
		return "", fmt.Errorf("write_file mkdir: %w", err)
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
		return "", fmt.Errorf("write_file %s: %w", p.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.Path), nil
}

// --- bash ---

type bashTool struct{}

func (*bashTool) Name() string { return "bash" }
func (*bashTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "bash",
			"description": "Execute a shell command and return its combined stdout+stderr output.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":   map[string]any{"type": "string", "description": "Shell command to execute."},
					"timeout_s": map[string]any{"type": "integer", "description": "Timeout in seconds (default 30)."},
				},
				"required": []string{"command"},
			},
		},
	})
}

func (*bashTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Command   string `json:"command"`
		TimeoutS  int    `json:"timeout_s"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bash: %w", err)
	}
	timeout := time.Duration(p.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("bash timeout after %v: %w", timeout, ctx.Err())
	}
	if err != nil {
		// Return output along with exit error so LLM can see stderr
		return buf.String(), fmt.Errorf("bash: %w\n%s", err, buf.String())
	}
	return buf.String(), nil
}

// helpers

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
