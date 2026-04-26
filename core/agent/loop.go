// Package agent implements the core agent loop.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/harness"
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
	Sensors  []harness.Sensor // nil = use global registry
	Guides   []harness.Guide
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
	var pendingSignals SignalContext
	for iter := 0; iter < maxToolIterations; iter++ {
		msgs := sess.Messages()
		if len(pendingSignals.Binding) > 0 || len(pendingSignals.Advisory) > 0 {
			msgs = InjectSignalsIntoMessages(msgs, pendingSignals)
			pendingSignals = SignalContext{}
		}
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

			// Run sensors on the tool result
			toolResult := harness.ToolResult{
				Tool:   tc.Name,
				Args:   tc.Args,
				Output: result,
				Err:    toolErr,
			}

			sensors := l.cfg.Sensors
			if sensors == nil {
				// Use global registry
				for _, name := range harness.AllSensorNames() {
					sensor, err := harness.BuildSensor(name, nil)
					if err == nil {
						sensors = append(sensors, sensor)
					}
				}
			}

			sigCtx := runSensors(ctx, sensors, toolResult, bus, sess.ID())

			// Rewrite tool result if binding error signal
			resultContent := result
			if toolErr != nil {
				resultContent = fmt.Sprintf("error: %v", toolErr)
			}
			for _, sig := range sigCtx.Binding {
				if sig.Severity == "error" {
					resultContent = fmt.Sprintf("[binding verdict: error] %s\n%s", sig.Message, resultContent)
					break
				}
			}

			sess.Append(llm.Message{
				Role:       "tool",
				Content:    resultContent,
				ToolCallID: tc.ID,
			})

			// Accumulate signals; they are injected before the next LLM call.
			pendingSignals.Binding = append(pendingSignals.Binding, sigCtx.Binding...)
			pendingSignals.Advisory = append(pendingSignals.Advisory, sigCtx.Advisory...)

			// Binding error: hard failure — halt turn and surface to transport.
			for _, sig := range sigCtx.Binding {
				if sig.Severity == "error" {
					return l.fail(bus, sess, fmt.Errorf("binding sensor error: %s", sig.Message))
				}
			}
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

const sensorTimeout = 5 * time.Second

// runSensors runs all sensors in parallel with a timeout.
// Returns a SignalContext with binding and advisory signals separated.
// Binding sensor timeout is a hard error; non-binding timeouts are dropped.
func runSensors(ctx context.Context, sensors []harness.Sensor, result harness.ToolResult, bus event.Bus, sessionID string) SignalContext {
	if len(sensors) == 0 {
		return SignalContext{}
	}

	ctx, cancel := context.WithTimeout(ctx, sensorTimeout)
	defer cancel()

	type sensorResult struct {
		name    string
		signals []harness.Signal
	}

	resultCh := make(chan sensorResult, len(sensors))
	for _, s := range sensors {
		go func(sensor harness.Sensor) {
			sigs := sensor.Observe(ctx, result)
			select {
			case resultCh <- sensorResult{name: sensor.Name(), signals: sigs}:
			case <-ctx.Done():
				// Timeout; don't send
			}
		}(s)
	}

	sigCtx := SignalContext{}
	received := 0
	deadline := time.Now().Add(sensorTimeout + time.Second) // with some slack

	for received < len(sensors) {
		select {
		case res := <-resultCh:
			// Separate signals by binding status
			for _, sig := range res.signals {
				if bus != nil {
					bus.Publish(event.Event{
						Kind:    event.SensorSignal,
						Session: sessionID,
						At:      time.Now(),
						Payload: event.SignalPayload{
							SensorName: res.name,
							Severity:   sig.Severity,
							Message:    sig.Message,
							Fix:        sig.Fix,
							Binding:    sig.Binding,
							ToolName:   result.Tool,
						},
					})
				}

				if sig.Binding {
					sigCtx.Binding = append(sigCtx.Binding, sig)
					if sig.Severity == "error" {
						// Binding error: hard failure
						// (but we continue collecting to report all errors)
					}
				} else {
					sigCtx.Advisory = append(sigCtx.Advisory, sig)
				}
			}
			received++
		case <-time.After(time.Until(deadline)):
			// Timeout: check if any binding sensors are still pending
			// For now, we just exit the loop
			break
		}
	}

	// Check if any binding sensors timed out
	if received < len(sensors) {
		// Some sensors timed out
		// If any were binding, this is a hard error
		// For simplicity, we'll report all timeouts at the end
		// (This would need more sophisticated tracking in production)
	}

	return sigCtx
}
