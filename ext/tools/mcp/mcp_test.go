package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/tools/mcp"
)

// fakeMCPServer simulates an MCP stdio server.
// It writes a tools/list response then echoes back tool/call results.
type fakeMCPServer struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newFakeMCPServer() (*fakeMCPServer, io.Reader, io.Writer) {
	// server → client pipe
	sr, sw := io.Pipe()
	// client → server pipe
	cr, cw := io.Pipe()
	f := &fakeMCPServer{r: cr, w: sw}
	go f.serve()
	return f, sr, cw
}

func (f *fakeMCPServer) serve() {
	dec := json.NewDecoder(f.r)
	enc := json.NewEncoder(f.w)
	for {
		var req map[string]any
		if err := dec.Decode(&req); err != nil {
			return
		}
		method, _ := req["method"].(string)
		id := req["id"]
		switch method {
		case "tools/list":
			enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "Echo the input",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"text": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "echo: " + text},
					},
				},
			})
		}
	}
}

func TestMCPDiscoverTools(t *testing.T) {
	t.Parallel()
	_, serverOut, serverIn := newFakeMCPServer()
	client, err := mcp.NewClient(serverOut, serverIn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	tools, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	// Schema should be in OpenAI function format.
	var schema map[string]any
	if err := json.Unmarshal(tools[0], &schema); err != nil {
		t.Fatalf("tool schema is not valid JSON: %v", err)
	}
	if schema["type"] != "function" {
		t.Errorf("expected type=function, got %v", schema["type"])
	}
	fn, _ := schema["function"].(map[string]any)
	if fn["name"] != "echo" {
		t.Errorf("expected name=echo, got %v", fn["name"])
	}
}

func TestMCPCallTool(t *testing.T) {
	t.Parallel()
	_, serverOut, serverIn := newFakeMCPServer()
	client, err := mcp.NewClient(serverOut, serverIn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := client.Call(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected result to contain 'hello', got %q", result)
	}
}
