// Package openai implements an OpenAI-compatible streaming LLM provider.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/tvmaly/nanogo/core/llm"
)

// Config holds the configuration for the OpenAI-compatible provider.
type Config struct {
	BaseURL     string           `json:"base_url"`
	APIKey      string           `json:"api_key"`
	APIKeyEnv   string           `json:"api_key_env"` // env var name; used by factory
	Model       string           `json:"model"`
	ServerTools []llm.ToolSchema `json:"server_tools"`
}

// Provider is an OpenAI-compatible streaming LLM client.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New constructs a Provider from a Config.
func New(cfg Config) *Provider {
	return &Provider{cfg: cfg, client: &http.Client{}}
}

func init() {
	llm.Register("openai", func(raw json.RawMessage) (llm.Provider, error) {
		var cfg Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		if cfg.APIKey == "" && cfg.APIKeyEnv != "" {
			cfg.APIKey = os.Getenv(cfg.APIKeyEnv)
		}
		return New(cfg), nil
	})
}

// --- wire types ---

type chatRequest struct {
	Model         string           `json:"model"`
	Messages      []wireMsg        `json:"messages"`
	Tools         []llm.ToolSchema `json:"tools,omitempty"`
	Stream        bool             `json:"stream"`
	StreamOptions map[string]bool  `json:"stream_options,omitempty"`
}

type wireMsg struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolCalls  []wireMsgToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type wireMsgToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Role      string         `json:"role"`
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

type wireToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireUsage struct {
	PromptTokens        int            `json:"prompt_tokens"`
	CompletionTokens    int            `json:"completion_tokens"`
	InputTokens         int            `json:"input_tokens"`
	OutputTokens        int            `json:"output_tokens"`
	CachedInputTokens   int            `json:"cached_input_tokens"`
	ServerToolUse       map[string]int `json:"server_tool_use"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// Chat sends a streaming chat completion request.
func (p *Provider) Chat(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	msgs := make([]wireMsg, len(req.Messages))
	for i, m := range req.Messages {
		wm := wireMsg{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireMsgToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: string(tc.Args)},
			})
		}
		msgs[i] = wm
	}

	tools := append([]llm.ToolSchema(nil), req.Tools...)
	tools = append(tools, p.cfg.ServerTools...)
	body, err := json.Marshal(chatRequest{
		Model:         model,
		Messages:      msgs,
		Tools:         tools,
		Stream:        true,
		StreamOptions: map[string]bool{"include_usage": true},
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ch := make(chan llm.Chunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.stream(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// stream reads SSE lines and sends Chunks.
func (p *Provider) stream(_ context.Context, r io.Reader, ch chan<- llm.Chunk) {
	// accumulate tool call arguments indexed by tool call index
	toolArgs := map[int]*strings.Builder{}
	toolMeta := map[int]*llm.ToolCall{}
	flushed := false // guard against multiple finish_reason chunks

	scanner := bufio.NewScanner(r)
	var lastUsage *wireUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var sc sseChunk
		if err := json.Unmarshal([]byte(data), &sc); err != nil {
			ch <- llm.Chunk{Err: fmt.Errorf("openai: parse error: %w", err)}
			continue
		}

		if sc.Usage != nil {
			lastUsage = sc.Usage
			if flushed {
				ch <- llm.Chunk{Usage: convertUsage(lastUsage)}
			}
		}

		if len(sc.Choices) == 0 {
			continue
		}
		choice := sc.Choices[0]

		// Text delta
		if choice.Delta.Content != "" {
			ch <- llm.Chunk{TextDelta: choice.Delta.Content}
		}

		// Tool call deltas
		for _, wtc := range choice.Delta.ToolCalls {
			idx := wtc.Index
			if _, ok := toolMeta[idx]; !ok {
				toolMeta[idx] = &llm.ToolCall{ID: wtc.ID, Name: wtc.Function.Name}
				toolArgs[idx] = &strings.Builder{}
			}
			if wtc.Function.Name != "" && toolMeta[idx].Name == "" {
				toolMeta[idx].Name = wtc.Function.Name
			}
			toolArgs[idx].WriteString(wtc.Function.Arguments)
		}

		// Finish
		if choice.FinishReason != nil && !flushed {
			flushed = true
			// flush tool calls
			for idx, tc := range toolMeta {
				tc.Args = json.RawMessage(toolArgs[idx].String())
				ch <- llm.Chunk{ToolCall: tc}
			}
			c := llm.Chunk{FinishReason: *choice.FinishReason}
			if lastUsage != nil {
				c.Usage = convertUsage(lastUsage)
			}
			ch <- c
		}
	}
}

func convertUsage(u *wireUsage) *llm.Usage {
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	if u.CachedInputTokens != 0 {
		cached = u.CachedInputTokens
	}
	input := u.PromptTokens
	if input == 0 {
		input = u.InputTokens
	}
	output := u.CompletionTokens
	if output == 0 {
		output = u.OutputTokens
	}
	return &llm.Usage{InputTokens: input, OutputTokens: output, CachedInputTokens: cached, ServerToolUse: u.ServerToolUse}
}
