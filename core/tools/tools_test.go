package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/tools"
)

// --- TEST-2.1: Tool registry and schema emission ---

func TestToolSchema(t *testing.T) {
	t.Parallel()
	src := tools.NewBuiltinSource(nil, nil, nil)
	list, err := src.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}

	want := map[string]bool{
		"read_file":  false,
		"write_file": false,
		"bash":       false,
		"ask_user":   false,
		"spawn":      false,
	}
	for _, tool := range list {
		want[tool.Name()] = true
		// verify OpenAI schema structure
		var schema struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
			t.Errorf("%s: invalid schema JSON: %v", tool.Name(), err)
			continue
		}
		if schema.Type != "function" {
			t.Errorf("%s: type = %q, want \"function\"", tool.Name(), schema.Type)
		}
		if schema.Function.Name != tool.Name() {
			t.Errorf("%s: function.name = %q, want %q", tool.Name(), schema.Function.Name, tool.Name())
		}
		if schema.Function.Description == "" {
			t.Errorf("%s: missing description", tool.Name())
		}
		var params struct {
			Type       string `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(schema.Function.Parameters, &params); err != nil {
			t.Errorf("%s: invalid parameters JSON: %v", tool.Name(), err)
		}
		if params.Type != "object" {
			t.Errorf("%s: parameters.type = %q, want \"object\"", tool.Name(), params.Type)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing builtin tool: %s", name)
		}
	}
}

// --- TEST-2.2: read_file behavior ---

func TestReadFile(t *testing.T) {
	t.Parallel()
	src := tools.NewBuiltinSource(nil, nil, nil)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{})
	rf := findTool(t, list, "read_file")

	// write a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// happy path
	out, err := rf.Call(context.Background(), jsonArgs(t, map[string]any{"path": path}))
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !strings.Contains(out, "line1") {
		t.Errorf("expected content, got: %q", out)
	}

	// missing file
	_, err = rf.Call(context.Background(), jsonArgs(t, map[string]any{"path": "/nonexistent/path/file.txt"}))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/file.txt") {
		t.Errorf("error should contain path, got: %v", err)
	}
}

// --- TEST-2.3: write_file behavior ---

func TestWriteFile(t *testing.T) {
	t.Parallel()
	src := tools.NewBuiltinSource(nil, nil, nil)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{})
	wf := findTool(t, list, "write_file")

	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "out.txt")

	// create new file (parent dir doesn't exist yet)
	out, err := wf.Call(context.Background(), jsonArgs(t, map[string]any{
		"path":    path,
		"content": "hello world",
	}))
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if !strings.Contains(out, path) {
		t.Errorf("output should contain path, got: %q", out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}

	// overwrite
	_, err = wf.Call(context.Background(), jsonArgs(t, map[string]any{
		"path":    path,
		"content": "overwritten",
	}))
	if err != nil {
		t.Fatalf("write_file overwrite: %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "overwritten" {
		t.Errorf("overwrite content = %q, want %q", got, "overwritten")
	}
}

// --- TEST-2.4: bash with timeout ---

func TestBash(t *testing.T) {
	t.Parallel()
	src := tools.NewBuiltinSource(nil, nil, nil)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{})
	bash := findTool(t, list, "bash")

	// basic command
	out, err := bash.Call(context.Background(), jsonArgs(t, map[string]any{"command": "echo hello"}))
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello', got: %q", out)
	}

	// timeout — use a 1-second timeout
	ctx := context.Background()
	_, err = bash.Call(ctx, jsonArgs(t, map[string]any{
		"command":   "sleep 10",
		"timeout_s": 1,
	}))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "signal") {
		t.Errorf("error should mention timeout/killed, got: %v", err)
	}
}

// --- TEST-2.5: ask_user publishes AskUser event and blocks ---

func TestAskUser(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := bus.Subscribe(ctx, event.AskUser)

	// Use a session coordinator that implements the WaitForAnswer contract
	coord := tools.NewAskUserCoordinator(bus, "sess-1")
	src := tools.NewBuiltinSource(bus, coord, nil)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{Session: "sess-1"})
	au := findTool(t, list, "ask_user")

	var (
		wg     sync.WaitGroup
		result string
		callErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, callErr = au.Call(ctx, jsonArgs(t, map[string]any{"question": "What is your name?"}))
	}()

	// wait for AskUser event
	select {
	case evt := <-sub:
		payload, ok := evt.Payload.(tools.AskUserPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", evt.Payload)
		}
		if payload.Question != "What is your name?" {
			t.Errorf("question = %q", payload.Question)
		}
		// Resume with an answer
		coord.Resume(payload.TurnID, "Alice")
	case <-ctx.Done():
		t.Fatal("timed out waiting for AskUser event")
	}

	wg.Wait()
	if callErr != nil {
		t.Fatalf("ask_user: %v", callErr)
	}
	if result != "Alice" {
		t.Errorf("result = %q, want %q", result, "Alice")
	}
}

// --- TEST-2.6: spawn creates isolated subagent session ---

func TestSpawn(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{result: "subagent done"}
	src := tools.NewBuiltinSource(nil, nil, runner)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{Session: "parent-session"})
	spawn := findTool(t, list, "spawn")

	out, err := spawn.Call(context.Background(), jsonArgs(t, map[string]any{
		"goal": "do something useful",
	}))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if out != "subagent done" {
		t.Errorf("spawn output = %q, want %q", out, "subagent done")
	}

	// session ID must differ from parent
	if runner.lastOpts.ParentSession == "" {
		t.Error("ParentSession not set")
	}
	if runner.lastOpts.Goal != "do something useful" {
		t.Errorf("Goal = %q", runner.lastOpts.Goal)
	}
}

// TEST-2.13: spawn honors model override
func TestSpawnModel(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{result: "done"}
	src := tools.NewBuiltinSource(nil, nil, runner)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{Session: "parent"})
	spawn := findTool(t, list, "spawn")

	_, err := spawn.Call(context.Background(), jsonArgs(t, map[string]any{
		"goal":  "task",
		"model": "cheap",
	}))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if runner.lastOpts.Model != "cheap" {
		t.Errorf("Model = %q, want %q", runner.lastOpts.Model, "cheap")
	}
}

// TEST-2.14: spawn honors role and tools allowlist
func TestSpawnRole(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{result: "done"}
	src := tools.NewBuiltinSource(nil, nil, runner)
	list, _ := src.Tools(context.Background(), tools.TurnInfo{Session: "parent"})
	spawn := findTool(t, list, "spawn")

	_, err := spawn.Call(context.Background(), jsonArgs(t, map[string]any{
		"goal":  "review",
		"role":  "code-reviewer",
		"tools": []string{"read_file", "bash"},
	}))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if runner.lastOpts.Role != "code-reviewer" {
		t.Errorf("Role = %q", runner.lastOpts.Role)
	}
	if len(runner.lastOpts.Tools) != 2 || runner.lastOpts.Tools[0] != "read_file" {
		t.Errorf("Tools = %v", runner.lastOpts.Tools)
	}

	// nonexistent role should produce error when runner returns error
	runner2 := &fakeRunner{err: fmt.Errorf("role not found: no-such-role")}
	src2 := tools.NewBuiltinSource(nil, nil, runner2)
	list2, _ := src2.Tools(context.Background(), tools.TurnInfo{Session: "parent"})
	spawn2 := findTool(t, list2, "spawn")
	_, err = spawn2.Call(context.Background(), jsonArgs(t, map[string]any{
		"goal": "task",
		"role": "no-such-role",
	}))
	if err == nil {
		t.Fatal("expected error for nonexistent role")
	}
}

// helpers

func findTool(t *testing.T, list []tools.Tool, name string) tools.Tool {
	t.Helper()
	for _, tool := range list {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in list", name)
	return nil
}

func jsonArgs(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

type fakeRunner struct {
	result   string
	err      error
	lastOpts tools.SubagentOpts
}

func (r *fakeRunner) RunSubagent(ctx context.Context, opts tools.SubagentOpts) (string, error) {
	r.lastOpts = opts
	return r.result, r.err
}
