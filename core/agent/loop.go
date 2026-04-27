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

type Config struct {
	Provider   llm.Provider
	Source     tools.Source
	Session    session.Session
	Bus        event.Bus
	Sensors    []harness.Sensor // nil = use global registry
	Guides     []harness.Guide
	Model      string
	SourceName string
	SkillName  string
	SubagentOf string
}

type Loop struct {
	cfg Config
}

func NewLoop(cfg Config) *Loop {
	return &Loop{cfg: cfg}
}

func (l *Loop) Run(ctx context.Context) error {
	sess := l.cfg.Session
	bus := l.cfg.Bus
	provider := l.cfg.Provider
	if l.cfg.SourceName != "" {
		ctx = context.WithValue(ctx, llm.CtxKeySource, l.cfg.SourceName)
	}
	if l.cfg.SkillName != "" {
		ctx = context.WithValue(ctx, llm.CtxKeySkill, l.cfg.SkillName)
	}

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
	var usage *llm.Usage
	var pendingSignals SignalContext
	for iter := 0; iter < maxToolIterations; iter++ {
		msgs := sess.Messages()
		if len(pendingSignals.Binding) > 0 || len(pendingSignals.Advisory) > 0 {
			msgs = InjectSignalsIntoMessages(msgs, pendingSignals)
			pendingSignals = SignalContext{}
		}
		req := buildRequest(msgs, toolList)
		req.Model = l.cfg.Model

		ch, err := provider.Chat(ctx, req)
		if err != nil {
			return l.fail(bus, sess, fmt.Errorf("agent llm: %w", err))
		}

		var (
			textBuf   strings.Builder
			toolCalls []llm.ToolCall
			finish    string
		)
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
					toolCalls = append(toolCalls, llm.ToolCall{ID: tc.ID, Name: tc.Name})
					tcArgs[tc.ID] = &strings.Builder{}
				}
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
			if chunk.Usage != nil {
				usage = chunk.Usage
			}
		}

		for i := range toolCalls {
			if b, ok := tcArgs[toolCalls[i].ID]; ok {
				raw := b.String()
				if raw == "" {
					raw = "{}"
				}
				toolCalls[i].Args = json.RawMessage(raw)
			}
		}

		if ctx.Err() != nil {
			return l.fail(bus, sess, ctx.Err())
		}

		if textBuf.Len() > 0 {
			text = textBuf.String()
		}

		if len(toolCalls) == 0 || finish == "stop" {
			sess.Append(llm.Message{Role: "assistant", Content: text})
			break
		}

		sess.Append(llm.Message{Role: "assistant", Content: text, ToolCalls: toolCalls})

		for _, tc := range toolCalls {
			result, toolErr := dispatchTool(ctx, tc, toolList, bus, sess.ID())

			toolResult := harness.ToolResult{
				Tool:   tc.Name,
				Args:   tc.Args,
				Output: result,
				Err:    toolErr,
			}

			sensors := l.cfg.Sensors
			if sensors == nil {
				for _, name := range harness.AllSensorNames() {
					sensor, err := harness.BuildSensor(name, nil)
					if err == nil {
						sensors = append(sensors, sensor)
					}
				}
			}

			sigCtx := runSensors(ctx, sensors, toolResult, bus, sess.ID())

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

			pendingSignals.Binding = append(pendingSignals.Binding, sigCtx.Binding...)
			pendingSignals.Advisory = append(pendingSignals.Advisory, sigCtx.Advisory...)

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
		payload := event.TurnCompletedPayload{
			Text:       text,
			Model:      l.cfg.Model,
			Source:     l.cfg.SourceName,
			Skill:      l.cfg.SkillName,
			SubagentOf: l.cfg.SubagentOf,
		}
		if usage != nil {
			payload.InputTokens = usage.InputTokens
			payload.OutputTokens = usage.OutputTokens
			payload.CachedInputTokens = usage.CachedInputTokens
			payload.ServerToolUse = usage.ServerToolUse
		}
		bus.Publish(event.Event{
			Kind:    event.TurnCompleted,
			Session: sess.ID(),
			At:      time.Now(),
			Payload: payload,
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
		Model:    "",
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
			}
		}(s)
	}

	sigCtx := SignalContext{}
	received := 0
	deadline := time.Now().Add(sensorTimeout + time.Second) // with some slack

	for received < len(sensors) {
		select {
		case res := <-resultCh:
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
					}
				} else {
					sigCtx.Advisory = append(sigCtx.Advisory, sig)
				}
			}
			received++
		case <-time.After(time.Until(deadline)):
			break
		}
	}

	if received < len(sensors) {
	}

	return sigCtx
}
