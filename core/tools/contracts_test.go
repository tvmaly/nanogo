package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/core/tools"
)

func TestContractRuntimeImplementsContracts(t *testing.T) {
	var _ contracts.ToolCatalog = (*tools.ContractRuntime)(nil)
	var _ contracts.ToolInvoker = (*tools.ContractRuntime)(nil)
	var _ contracts.ToolRuntime = (*tools.ContractRuntime)(nil)
}

func TestContractRuntimeListToolsMapsSourceTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"function","function":{"name":"echo","description":"Echo text","parameters":{"type":"object","properties":{"text":{"type":"string"}}}}}`)
	src := staticSource{tools: []tools.Tool{&contractTool{name: "echo", schema: schema}}}
	runtime := tools.NewContractRuntime(src)

	specs, err := runtime.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %d, want 1", len(specs))
	}
	if specs[0].Name != "echo" {
		t.Fatalf("Name = %q", specs[0].Name)
	}
	if specs[0].Description != "Echo text" {
		t.Fatalf("Description = %q", specs[0].Description)
	}
	if string(specs[0].InputSchema) != string(schema) {
		t.Fatalf("InputSchema = %s", specs[0].InputSchema)
	}
}

func TestContractRuntimeInvokeToolCallsNamedTool(t *testing.T) {
	tool := &contractTool{name: "echo", result: "hello"}
	runtime := tools.NewContractRuntime(staticSource{tools: []tools.Tool{tool}})

	got, err := runtime.InvokeTool(context.Background(), contracts.ToolInvocation{
		Name:      "echo",
		Arguments: map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("InvokeTool: %v", err)
	}
	if got.Text != "hello" {
		t.Fatalf("Text = %q", got.Text)
	}
	if len(tool.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(tool.calls))
	}
	var args map[string]string
	if err := json.Unmarshal(tool.calls[0], &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["text"] != "hello" {
		t.Fatalf("args = %+v", args)
	}
}

func TestContractRuntimeInvokeToolMissingTool(t *testing.T) {
	runtime := tools.NewContractRuntime(staticSource{})

	_, err := runtime.InvokeTool(context.Background(), contracts.ToolInvocation{Name: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestContractRuntimeInvokeToolPropagatesToolError(t *testing.T) {
	runtime := tools.NewContractRuntime(staticSource{tools: []tools.Tool{
		&contractTool{name: "boom", err: errors.New("boom")},
	}})

	got, err := runtime.InvokeTool(context.Background(), contracts.ToolInvocation{Name: "boom"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Error != "boom" {
		t.Fatalf("ToolResult.Error = %q", got.Error)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

type staticSource struct {
	tools []tools.Tool
	err   error
}

func (s staticSource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return s.tools, s.err
}

type contractTool struct {
	name   string
	schema json.RawMessage
	result string
	err    error
	calls  []json.RawMessage
}

func (t *contractTool) Name() string { return t.name }

func (t *contractTool) Schema() json.RawMessage {
	if len(t.schema) > 0 {
		return t.schema
	}
	return json.RawMessage(`{"type":"function","function":{"name":"` + t.name + `","description":"test tool","parameters":{"type":"object"}}}`)
}

func (t *contractTool) Call(_ context.Context, args json.RawMessage) (string, error) {
	t.calls = append(t.calls, append(json.RawMessage(nil), args...))
	return t.result, t.err
}
