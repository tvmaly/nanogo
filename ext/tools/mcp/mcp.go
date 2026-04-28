// Package mcp implements a minimal MCP (Model Context Protocol) client
// over stdio using JSON-RPC 2.0.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Client speaks MCP JSON-RPC over an io.Reader/Writer pair (stdin/stdout of
// the MCP server process).
type Client struct {
	enc *json.Encoder
	dec *json.Decoder
	mu  sync.Mutex
	seq atomic.Int64
}

// NewClient wraps the given reader (server stdout) and writer (server stdin).
func NewClient(r io.Reader, w io.Writer) (*Client, error) {
	return &Client{enc: json.NewEncoder(w), dec: json.NewDecoder(r)}, nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.seq.Add(1)
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	c.mu.Lock()
	if err := c.enc.Encode(req); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := c.dec.Decode(&resp); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type listResult struct {
	Tools []mcpTool `json:"tools"`
}

// Discover fetches the tool list and converts schemas to OpenAI function format.
func (c *Client) Discover(ctx context.Context) ([]json.RawMessage, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res listResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		}
		b, err := json.Marshal(schema)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

type callResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Call invokes a named tool with args and returns the text content.
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return "", err
	}
	raw, err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": argsMap,
	})
	if err != nil {
		return "", err
	}
	var res callResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	var sb string
	for _, c := range res.Content {
		if c.Type == "text" {
			sb += c.Text
		}
	}
	return sb, nil
}
