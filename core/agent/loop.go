// Package agent implements the core agent loop.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
)

const maxToolIterations = 200

// Config configures a Loop.
type Config struct {
	Provider llm.Provider
	Source   tools.Source
	Session  session.Session
	Bus      event.Bus
}

// Loop runs the agent turn loop.
type Loop struct {
	cfg Config
}

// NewLoop constructs a Loop from a Config.
func NewLoop(cfg Config) *Loop {
	return &Loop{cfg: cfg}
}

// Run executes the agent loop until the LLM stops or an error occurs.
// The caller must have appended the user message to the session before calling Run.
func (l *Loop) Run(ctx context.Context) error {
	sess := l.cfg.Session
	bus := l.cfg.Bus
	provider := l.cfg.Provider

	turn := sess.Messages()
	if len(turn) == 0 {
		return fmt.Errorf("agent: session has no messages")
	}

	if bus != nil {
		bus.Publish(event.Event{
			Kind:    event.TurnStarted,
			Session: sess.ID(),
			At:      time.Now(),
		})
	}

	toolSource := l.cfg.Source
	turnInfo := tools.TurnInfo{Session: sess.ID(), Turn: len(turn)}
	toolList, err := toolSource.Tools(ctx, turnInfo)
	if err != nil {
		return l.fail(bus, sess, fmt.Errorf("agent tools: %w", err))
	}

	var text string
	for iter := 0; iter < maxToolIterations; iter++ {
		msgs := sess.Messages()
		req := buildRequest(msgs, toolList)

		ch, err := provider.Chat(ctx, req)
		if err != nil {
			return l.fail(bus, sess, fmt.Errorf("agent llm: %w", err))
		}

		// Collect all chunks from this response
		var (
			textBuf   strings.Builder
			toolCalls []llm.ToolCall
			finish    string
		)
		// Track partial tool call args across chunks
		tcArgs := map[string]*strings.Builder{}

		for chunk := range ch {
			if chunk.Err != nil {
				return l.fail(bus, sess, fmt.Errorf("agent stream: %w", chunk.Err))
			}
			if ctx.Err() != nil {
				return l.fail(bus, sess, ctx.Err())
			}
			if chunk.TextDelta != "" {
				textBuf.WriteString(chunk.TextDelta)
				if bus != nil {
					bus.Publish(event.Event{
						Kind: event.TokenDelta, Session: sess.ID(),
						Payload: chunk.TextDelta, At: time.Now(),
					})
				}
			}
			if chunk.ToolCall != nil {
				tc := chunk.ToolCall
				if tc.ID != "" {
					// New tool call
					toolCalls = append(toolCalls, llm.ToolCall{ID: tc.ID, Name: tc.Name})
					tcArgs[tc.ID] = &strings.Builder{}
				}
				// Accumulate args (may come across multiple chunks)
				if len(tcArgs) > 0 && len(toolCalls) > 0 {
					last := toolCalls[len(toolCalls)-1]
					if b, ok := tcArgs[last.ID]; ok {
						if len(tc.Args) > 0 {
							b.Write(tc.Args)
						}
					}
				}
			}
			if chunk.FinishReason != "" {
				finish = chunk.FinishReason
			}
		}

		// Finalize tool call args
		for i := range toolCalls {
			if b, ok := tcArgs[toolCalls[i].ID]; ok {
				raw := b.String()
				if raw == "" {
					raw = "{}"
				}
				toolCalls[i].Args = json.RawMessage(raw)
			}
		}

		// Check if context was cancelled during streaming
		if ctx.Err() != nil {
			return l.fail(bus, sess, ctx.Err())
		}

		if textBuf.Len() > 0 {
			text = textBuf.String()
		}

		if len(toolCalls) == 0 || finish == "stop" {
			// Final assistant message
			sess.Append(llm.Message{Role: "assistant", Content: text})
			break
		}

		// Append assistant message with tool calls (preserve any text content too)
		sess.Append(llm.Message{Role: "assistant", Content: text, ToolCalls: toolCalls})

		// Execute each tool call
		for _, tc := range toolCalls {
			result, toolErr := dispatchTool(ctx, tc, toolList, bus, sess.ID())
			resultContent := result
			if toolErr != nil {
				resultContent = fmt.Sprintf("error: %v", toolErr)
			}
			sess.Append(llm.Message{
				Role:       "tool",
				Content:    resultContent,
				ToolCallID: tc.ID,
			})
		}

		if iter == maxToolIterations-1 {
			return l.fail(bus, sess, fmt.Errorf("agent: max tool iterations (%d) exceeded", maxToolIterations))
		}
	}

	_ = sess.Save()

	if bus != nil {
		bus.Publish(event.Event{
			Kind:    event.TurnCompleted,
			Session: sess.ID(),
			At:      time.Now(),
			Payload: event.TurnCompletedPayload{Text: text},
		})
	}
	return nil
}

func (l *Loop) fail(bus event.Bus, sess session.Session, err error) error {
	if bus != nil {
		bus.Publish(event.Event{
			Kind:    event.Error,
			Session: sess.ID(),
			At:      time.Now(),
			Payload: err.Error(),
		})
	}
	return err
}

func buildRequest(msgs []llm.Message, toolList []tools.Tool) llm.Request {
	schemas := make([]llm.ToolSchema, len(toolList))
	for i, t := range toolList {
		schemas[i] = t.Schema()
	}
	return llm.Request{
		Messages: msgs,
		Tools:    schemas,
		Stream:   true,
	}
}

func dispatchTool(ctx context.Context, tc llm.ToolCall, toolList []tools.Tool, bus event.Bus, sessionID string) (string, error) {
	if bus != nil {
		bus.Publish(event.Event{
			Kind: event.ToolCallStarted, Session: sessionID, At: time.Now(),
			Payload: map[string]string{"tool": tc.Name, "id": tc.ID},
		})
	}

	var result string
	var err error
	var found bool
	for _, t := range toolList {
		if t.Name() == tc.Name {
			found = true
			result, err = t.Call(ctx, tc.Args)
			break
		}
	}
	if !found {
		err = fmt.Errorf("unknown tool: %s", tc.Name)
	}

	if bus != nil {
		bus.Publish(event.Event{
			Kind: event.ToolCallResult, Session: sessionID, At: time.Now(),
			Payload: map[string]string{"tool": tc.Name, "result": result},
		})
	}
	return result, err
}
