package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/ext/llm/openai"
)

func serveSSE(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(data)
	}))
}

func TestBasicStream(t *testing.T) {
	t.Parallel()
	srv := serveSSE(t, "openai_basic.sse")
	defer srv.Close()

	p := openai.New(openai.Config{
		BaseURL: srv.URL,
		APIKey:  "test",
		Model:   "test-model",
	})

	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}

	var text strings.Builder
	var finishReason string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		text.WriteString(chunk.TextDelta)
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}

	if text.String() != "Hello, world" {
		t.Fatalf("expected 'Hello, world', got %q", text.String())
	}
	if finishReason != "stop" {
		t.Fatalf("expected finish_reason 'stop', got %q", finishReason)
	}
}

func TestToolCallParsing(t *testing.T) {
	t.Parallel()
	srv := serveSSE(t, "openai_tool_call.sse")
	defer srv.Close()

	p := openai.New(openai.Config{BaseURL: srv.URL, APIKey: "test", Model: "test-model"})
	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}

	var toolCall *llm.ToolCall
	var textDelta string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		if chunk.ToolCall != nil {
			toolCall = chunk.ToolCall
		}
		textDelta += chunk.TextDelta
	}

	if toolCall == nil {
		t.Fatal("expected a tool call chunk")
	}
	if toolCall.Name != "read_file" {
		t.Fatalf("expected name read_file, got %q", toolCall.Name)
	}
	if string(toolCall.Args) != `{"path":"foo.txt"}` {
		t.Fatalf("unexpected args: %s", toolCall.Args)
	}
	if textDelta != "" {
		t.Fatalf("expected no text delta for tool-call stream, got %q", textDelta)
	}
}

func TestErrors_HTTP429(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"rate limit"}}`))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{BaseURL: srv.URL, APIKey: "test", Model: "test-model"})
	_, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 in error, got %q", err.Error())
	}
}

func TestErrors_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := serveSSE(t, "openai_error.sse")
	defer srv.Close()

	p := openai.New(openai.Config{BaseURL: srv.URL, APIKey: "test", Model: "test-model"})
	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}

	var errChunk *llm.Chunk
	for chunk := range ch {
		if chunk.Err != nil {
			c := chunk
			errChunk = &c
		}
	}
	if errChunk == nil {
		t.Fatal("expected an error chunk for malformed JSON")
	}
}

func TestUsagePopulated(t *testing.T) {
	t.Parallel()
	srv := serveSSE(t, "openai_with_usage.sse")
	defer srv.Close()

	p := openai.New(openai.Config{BaseURL: srv.URL, APIKey: "test", Model: "test-model"})
	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}

	var lastChunk llm.Chunk
	for chunk := range ch {
		lastChunk = chunk
	}

	if lastChunk.Usage == nil {
		t.Fatal("expected Usage on final chunk")
	}
	if lastChunk.Usage.InputTokens != 10 {
		t.Fatalf("InputTokens = %d, want 10", lastChunk.Usage.InputTokens)
	}
	if lastChunk.Usage.OutputTokens != 5 {
		t.Fatalf("OutputTokens = %d, want 5", lastChunk.Usage.OutputTokens)
	}
	if lastChunk.Usage.CachedInputTokens != 2 {
		t.Fatalf("CachedInputTokens = %d, want 2", lastChunk.Usage.CachedInputTokens)
	}
}

// TEST-10.9 — server tools appended without removing user-defined tools
func TestOpenAIServerToolsPassthrough(t *testing.T) {
	t.Parallel()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	serverTool := json.RawMessage(`{"type":"openrouter:web_search","parameters":{"engine":"auto"}}`)
	p := openai.New(openai.Config{
		BaseURL:     srv.URL,
		APIKey:      "test",
		Model:       "openai/gpt-5.2",
		ServerTools: []json.RawMessage{serverTool},
	})

	userTool := json.RawMessage(`{"type":"function","function":{"name":"my_fn"}}`)
	ch, err := p.Chat(context.Background(), llm.Request{
		Stream: true,
		Tools:  []llm.ToolSchema{userTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}

	var body map[string]any
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("request body not valid JSON: %v", err)
	}

	tools, _ := body["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools in request, got %d", len(tools))
	}

	foundFunction, foundServerTool := false, false
	for _, tool := range tools {
		m, _ := tool.(map[string]any)
		switch m["type"] {
		case "function":
			foundFunction = true
		case "openrouter:web_search":
			foundServerTool = true
		}
	}
	if !foundFunction {
		t.Error("user-defined function tool missing from request")
	}
	if !foundServerTool {
		t.Error("openrouter:web_search server tool missing from request")
	}

	// model must not include :online
	model, _ := body["model"].(string)
	if strings.Contains(model, ":online") {
		t.Errorf("model must not contain :online, got %q", model)
	}
	// no plugins field
	if _, ok := body["plugins"]; ok {
		t.Error("request must not contain plugins field")
	}
}

// TEST-10.10 — web-search server tool parameters are raw JSON pass-through
func TestOpenAIWebSearchServerToolParameters(t *testing.T) {
	t.Parallel()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	serverTool := json.RawMessage(`{"type":"openrouter:web_search","parameters":{"engine":"auto","max_results":5,"max_total_results":15,"search_context_size":"medium","allowed_domains":["nasa.gov","si.edu"],"excluded_domains":["reddit.com"]}}`)
	p := openai.New(openai.Config{
		BaseURL:     srv.URL,
		APIKey:      "test",
		Model:       "openai/gpt-5.2",
		ServerTools: []json.RawMessage{serverTool},
	})

	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}

	var body struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("request body not valid JSON: %v", err)
	}
	if len(body.Tools) != 1 {
		t.Fatalf("expected one server tool, got %d", len(body.Tools))
	}

	var got map[string]any
	if err := json.Unmarshal(body.Tools[0], &got); err != nil {
		t.Fatalf("server tool not valid JSON: %v", err)
	}
	if got["type"] != "openrouter:web_search" {
		t.Fatalf("type = %v, want openrouter:web_search", got["type"])
	}
	params, ok := got["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or wrong type: %#v", got["parameters"])
	}
	checks := map[string]any{
		"engine":              "auto",
		"max_results":         float64(5),
		"max_total_results":   float64(15),
		"search_context_size": "medium",
	}
	for k, want := range checks {
		if params[k] != want {
			t.Fatalf("parameters.%s = %#v, want %#v", k, params[k], want)
		}
	}
	if gotDomains := fmt.Sprint(params["allowed_domains"]); gotDomains != "[nasa.gov si.edu]" {
		t.Fatalf("allowed_domains = %s", gotDomains)
	}
	if gotDomains := fmt.Sprint(params["excluded_domains"]); gotDomains != "[reddit.com]" {
		t.Fatalf("excluded_domains = %s", gotDomains)
	}
}

// TEST-10.13 — general server-tool search remains unrestricted unless configured
func TestGeneralServerToolSearchUnrestricted(t *testing.T) {
	t.Parallel()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	serverTool := json.RawMessage(`{"type":"openrouter:web_search","parameters":{"engine":"auto","max_results":5,"max_total_results":20,"search_context_size":"medium"}}`)
	p := openai.New(openai.Config{
		BaseURL:     srv.URL,
		APIKey:      "test",
		Model:       "openai/gpt-5.2",
		ServerTools: []json.RawMessage{serverTool},
	})

	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}

	var body struct {
		Tools []struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(captured, &body); err != nil {
		t.Fatalf("request body not valid JSON: %v", err)
	}
	if len(body.Tools) != 1 {
		t.Fatalf("expected one server tool, got %d", len(body.Tools))
	}
	params := body.Tools[0].Parameters
	if _, ok := params["allowed_domains"]; ok {
		t.Fatalf("allowed_domains must be absent unless configured: %#v", params)
	}
	if _, ok := params["excluded_domains"]; ok {
		t.Fatalf("excluded_domains must be absent unless configured: %#v", params)
	}
}

// TEST-10.11 — server_tool_use parsed from usage
func TestOpenAIServerToolUsage(t *testing.T) {
	t.Parallel()
	sse := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"prompt_tokens_details":{"cached_tokens":100},"server_tool_use":{"web_search_requests":2}}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()

	p := openai.New(openai.Config{BaseURL: srv.URL, APIKey: "test", Model: "m"})
	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	var last llm.Chunk
	for c := range ch {
		if c.Usage != nil {
			last = c
		}
	}
	if last.Usage == nil {
		t.Fatal("expected Usage on final chunk")
	}
	if last.Usage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", last.Usage.InputTokens)
	}
	if last.Usage.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", last.Usage.OutputTokens)
	}
	if last.Usage.CachedInputTokens != 100 {
		t.Errorf("CachedInputTokens = %d, want 100", last.Usage.CachedInputTokens)
	}
	if last.Usage.ServerToolUse["web_search_requests"] != 2 {
		t.Errorf("web_search_requests = %d, want 2", last.Usage.ServerToolUse["web_search_requests"])
	}
}

// TEST-10.12 — deprecated :online and plugins are never emitted
func TestOpenRouterDeprecatedWebSearchNotUsed(t *testing.T) {
	t.Parallel()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	serverTool := json.RawMessage(`{"type":"openrouter:web_search","parameters":{"engine":"auto"}}`)
	p := openai.New(openai.Config{
		BaseURL:     srv.URL,
		APIKey:      "test",
		Model:       "some/model",
		ServerTools: []json.RawMessage{serverTool},
	})
	ch, err := p.Chat(context.Background(), llm.Request{Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}

	var body map[string]any
	json.Unmarshal(captured, &body)
	model, _ := body["model"].(string)
	if strings.Contains(model, ":online") {
		t.Errorf("model must not contain :online, got %q", model)
	}
	if _, ok := body["plugins"]; ok {
		t.Error("plugins field must not be present")
	}
}
