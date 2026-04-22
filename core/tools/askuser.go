package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tvmaly/nanogo/core/event"
)

// AskUserPayload is the payload of an AskUser bus event.
type AskUserPayload struct {
	TurnID   string
	Question string
}

// AskUserCoord manages the ask_user blocking/resume cycle for a session.
type AskUserCoord interface {
	// Ask publishes an AskUser event and blocks until Resume is called with the matching turnID.
	Ask(ctx context.Context, sessionID, question string) (string, error)
	// Resume delivers an answer to a pending Ask call identified by turnID.
	Resume(turnID, answer string)
}

// askUserCoordinator is the default AskUserCoord implementation.
type askUserCoordinator struct {
	bus     event.Bus
	session string

	mu      sync.Mutex
	pending map[string]chan string
	nextID  int
}

// NewAskUserCoordinator creates an AskUserCoord backed by the given bus.
func NewAskUserCoordinator(bus event.Bus, sessionID string) *askUserCoordinator {
	return &askUserCoordinator{
		bus:     bus,
		session: sessionID,
		pending: make(map[string]chan string),
	}
}

func (c *askUserCoordinator) Ask(ctx context.Context, sessionID, question string) (string, error) {
	c.mu.Lock()
	c.nextID++
	turnID := fmt.Sprintf("ask-%d", c.nextID)
	ch := make(chan string, 1)
	c.pending[turnID] = ch
	c.mu.Unlock()

	c.bus.Publish(event.Event{
		Kind:    event.AskUser,
		Session: sessionID,
		Payload: AskUserPayload{TurnID: turnID, Question: question},
	})

	select {
	case answer := <-ch:
		return answer, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, turnID)
		c.mu.Unlock()
		return "", ctx.Err()
	}
}

func (c *askUserCoordinator) Resume(turnID, answer string) {
	c.mu.Lock()
	ch, ok := c.pending[turnID]
	if ok {
		delete(c.pending, turnID)
	}
	c.mu.Unlock()
	if ok {
		ch <- answer
	}
}

// --- ask_user tool ---

type askUserTool struct {
	coord AskUserCoord
}

func newAskUserTool(coord AskUserCoord) *askUserTool {
	return &askUserTool{coord: coord}
}

func (*askUserTool) Name() string { return "ask_user" }
func (*askUserTool) Schema() json.RawMessage {
	return mustJSON(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "ask_user",
			"description": "Ask the user a question and wait for their reply before continuing.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{"type": "string", "description": "The question to ask the user."},
				},
				"required": []string{"question"},
			},
		},
	})
}

func (t *askUserTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.coord == nil {
		return "", fmt.Errorf("ask_user: no coordinator configured")
	}
	var p struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}
	// The coordinator's Ask method uses its own sessionID field.
	// We pass the session via the coord which already has it.
	return t.coord.Ask(ctx, "", p.Question)
}
