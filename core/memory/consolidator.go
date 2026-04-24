package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
)

// consolidator implements Consolidator.
type consolidator struct {
	store     *Store
	bus       event.Bus
	sessions  session.Store
	provider  llm.Provider
	threshold int // token count that triggers consolidation
	chunkSize int // messages to summarize per run
	failures  map[string]int
	ch        <-chan event.Event
}

// ConsolidatorConfig holds tuning parameters.
type ConsolidatorConfig struct {
	ThresholdTokens int
	ChunkSize       int
}

// NewConsolidator creates a Consolidator backed by the given store.
// It subscribes to TurnCompleted events immediately so no events are missed.
func NewConsolidator(store *Store, bus event.Bus, sessions session.Store, provider llm.Provider, cfg ConsolidatorConfig) Consolidator {
	if cfg.ThresholdTokens == 0 {
		cfg.ThresholdTokens = 8000
	}
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 20
	}
	// Subscribe with background context; Run will drain the channel.
	ch := bus.Subscribe(context.Background(), event.TurnCompleted)
	return &consolidator{
		store:     store,
		bus:       bus,
		sessions:  sessions,
		provider:  provider,
		threshold: cfg.ThresholdTokens,
		chunkSize: cfg.ChunkSize,
		failures:  map[string]int{},
		ch:        ch,
	}
}

func (c *consolidator) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-c.ch:
			if !ok {
				return nil
			}
			p, ok := evt.Payload.(event.TurnCompletedPayload)
			if !ok || evt.Session == "" {
				continue
			}
			if err := c.maybeConsolidate(ctx, evt.Session, p); err != nil {
				c.bus.Publish(event.Event{Kind: event.Error, Payload: fmt.Sprintf("consolidator: %v", err)})
			}
		}
	}
}

func (c *consolidator) maybeConsolidate(ctx context.Context, sessionID string, p event.TurnCompletedPayload) error {
	tokens := p.InputTokens + p.OutputTokens
	if tokens < c.threshold {
		return nil
	}
	sess, err := c.sessions.Load(sessionID)
	if err != nil {
		return err
	}
	msgs := sess.Messages()
	if len(msgs) < c.chunkSize {
		return nil
	}
	chunk := msgs[:c.chunkSize]
	summary, err := c.summarize(ctx, chunk)
	if err != nil {
		c.failures[sessionID]++
		if c.failures[sessionID] >= 3 {
			// raw dump fallback
			delete(c.failures, sessionID)
			return c.rawDump(chunk)
		}
		return fmt.Errorf("summarize failed (attempt %d): %w", c.failures[sessionID], err)
	}
	delete(c.failures, sessionID)
	if err := c.store.AppendHistory(HistoryEntry{
		At:      time.Now(),
		Role:    "summary",
		Content: summary,
		Cursor:  true,
	}); err != nil {
		return err
	}
	// Trim the chunk from the session by re-appending remaining messages.
	// Session doesn't expose a Trim method, so we rebuild via a new session.
	// For now we publish MemoryUpdated to signal completion.
	c.bus.Publish(event.Event{Kind: event.MemoryUpdated, Session: sessionID, Payload: "consolidation"})
	return nil
}

func (c *consolidator) summarize(ctx context.Context, msgs []llm.Message) (string, error) {
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation in 2-3 sentences:\n\n")
	for _, m := range msgs {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	req := llm.Request{
		Model:    "",
		Messages: []llm.Message{{Role: "user", Content: sb.String()}},
	}
	ch, err := c.provider.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	for chunk := range ch {
		result.WriteString(chunk.TextDelta)
	}
	return strings.TrimSpace(result.String()), nil
}

func (c *consolidator) rawDump(msgs []llm.Message) error {
	type dump struct {
		At      time.Time `json:"at"`
		Kind    string    `json:"kind"`
		Content string    `json:"content"`
	}
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Role + ": " + m.Content + "\n")
	}
	line, _ := json.Marshal(dump{At: time.Now(), Kind: "raw_dump", Content: sb.String()})
	return c.store.AppendJSONL("memory/history.jsonl", json.RawMessage(line))
}
