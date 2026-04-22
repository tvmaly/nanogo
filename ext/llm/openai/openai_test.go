package openai_test

import (
	"context"
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
