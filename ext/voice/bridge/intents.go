package bridge

import "context"

type Intent struct {
	Type    string
	ChildID string
	Subject string
	Topic   string
	Text    string
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
	flow AgentFlow
}

func New(flow AgentFlow) *Bridge {
	return &Bridge{flow: flow}
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
	default:
		if b.flow == nil {
			return "I recorded that request.", nil, nil
		}
		text, err := b.flow.Submit(ctx, intent)
		return text, nil, err
	}
}
