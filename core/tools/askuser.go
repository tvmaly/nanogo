package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/session"
)

// AskUserPayload is the payload of an AskUser bus event.
type AskUserPayload struct{ TurnID, Question string }

// AskUserCoord manages the ask_user blocking/resume cycle for a session.
type AskUserCoord interface {
	Ask(ctx context.Context, sessionID, question string) (string, error)
	Resume(turnID, answer string)
}

type askUserCoordinator struct {
	bus     event.Bus
	sess    session.Session
	mu      sync.Mutex
	nextID  int
	pending map[string]chan string
}

// NewAskUserCoordinator creates a coordinator without session integration.
func NewAskUserCoordinator(bus event.Bus, _ string) *askUserCoordinator {
	return &askUserCoordinator{bus: bus, pending: make(map[string]chan string)}
}

// NewAskUserCoordinatorWithSession creates a coordinator that checkpoints
// the session to StatusWaiting, enabling pause/resume across restarts.
func NewAskUserCoordinatorWithSession(bus event.Bus, sess session.Session) *askUserCoordinator {
	return &askUserCoordinator{bus: bus, sess: sess, pending: make(map[string]chan string)}
}

func (c *askUserCoordinator) Ask(ctx context.Context, sessionID, question string) (string, error) {
	c.mu.Lock()
	c.nextID++
	turnID := fmt.Sprintf("ask-%d", c.nextID)
	c.mu.Unlock()

	var ch <-chan string
	if c.sess != nil {
		ch = c.sess.SetWaiting(turnID)
	} else {
		lch := make(chan string, 1)
		c.mu.Lock(); c.pending[turnID] = lch; c.mu.Unlock()
		ch = lch
	}
	c.bus.Publish(event.Event{Kind: event.AskUser, Session: sessionID,
		Payload: AskUserPayload{TurnID: turnID, Question: question}})
	select {
	case answer := <-ch:
		return answer, nil
	case <-ctx.Done():
		if c.sess == nil { c.mu.Lock(); delete(c.pending, turnID); c.mu.Unlock() }
		return "", ctx.Err()
	}
}

func (c *askUserCoordinator) Resume(turnID, answer string) {
	if c.sess != nil { c.sess.Resume(turnID, answer); return }
	c.mu.Lock()
	ch, ok := c.pending[turnID]
	if ok { delete(c.pending, turnID) }
	c.mu.Unlock()
	if ok { ch <- answer }
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
