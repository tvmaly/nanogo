package bridge

import (
	"context"
	"fmt"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Intent struct {
	Type    string
	ChildID string
	Subject string
	Topic   string
	Text    string
	Payload map[string]any
}

type Action struct {
	Type    string
	ChildID string
	Topic   string
	Payload map[string]any
}

type AgentFlow interface {
	Submit(ctx context.Context, intent Intent) (string, error)
}

type Bridge struct {
	flow    AgentFlow
	pattern contracts.PatternRunner
	handoff contracts.HandoffTarget
}

func New(flow AgentFlow) *Bridge {
	return &Bridge{flow: flow}
}

func NewPatternBridge(pattern contracts.PatternRunner, handoff contracts.HandoffTarget) *Bridge {
	return &Bridge{pattern: pattern, handoff: handoff}
}

func (b *Bridge) Handle(ctx context.Context, intent Intent) (string, *Action, error) {
	switch intent.Type {
	case "child_show_video_request", "child_build_game_request":
		return "I will put that on the screen.", &Action{
			Type:    intent.Type,
			ChildID: intent.ChildID,
			Topic:   intent.Topic,
			Payload: map[string]any{"text": intent.Text},
		}, nil
	case "handoff":
		if b.handoff == nil {
			return "", nil, fmt.Errorf("voice bridge: no handoff target configured")
		}
		out, err := b.handoff.Handoff(ctx, contracts.HandoffInput{
			ToAgent: stringPayload(intent.Payload, "to_agent"), Reason: intent.Type, Summary: intent.Text, Prompt: intent.Text,
			Context: map[string]any{"child_id": intent.ChildID, "subject": intent.Subject, "topic": intent.Topic},
			Policy:  contracts.PatternPolicy{AllowHandoff: true, AllowedAgents: []string{stringPayload(intent.Payload, "to_agent")}},
		})
		return out.Text, nil, err
	default:
		if b.pattern != nil {
			out, err := b.pattern.RunPattern(ctx, contracts.PatternRequest{
				StudentID: intent.ChildID, Prompt: intent.Text, PatternHint: "single",
				Context: map[string]any{"subject": intent.Subject, "topic": intent.Topic, "intent_type": intent.Type},
			})
			return out.Text, nil, err
		}
		if b.flow == nil {
			return "I recorded that request.", nil, nil
		}
		text, err := b.flow.Submit(ctx, intent)
		return text, nil, err
	}
}

func stringPayload(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}
