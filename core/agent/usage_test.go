package agent_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	fakeevent "github.com/tvmaly/nanogo/core/event/fake"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	fakesession "github.com/tvmaly/nanogo/core/session/fake"
	faketools "github.com/tvmaly/nanogo/core/tools/fake"
)

func TestLoopPublishesUsageMetadata(t *testing.T) {
	t.Parallel()

	provider := fakellm.New([]llm.Chunk{
		{TextDelta: "ok"},
		{FinishReason: "stop", Usage: &llm.Usage{
			InputTokens:       1000,
			OutputTokens:      500,
			CachedInputTokens: 200,
		}},
	})
	bus := fakeevent.New()
	sess := fakesession.New("usage-session")
	sess.Append(llm.Message{Role: "user", Content: "hello"})

	loop := agent.NewLoop(agent.Config{
		Provider:   provider,
		Source:     faketools.NewSource(),
		Session:    sess,
		Bus:        bus,
		Model:      "model-a",
		SourceName: "cli",
		SkillName:  "lesson",
		SubagentOf: "parent-1",
	})
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got event.TurnCompletedPayload
	for _, e := range bus.Events() {
		if e.Kind == event.TurnCompleted {
			var ok bool
			got, ok = e.Payload.(event.TurnCompletedPayload)
			if !ok {
				t.Fatalf("payload type = %T", e.Payload)
			}
		}
	}
	if got.Model != "model-a" || got.Source != "cli" || got.Skill != "lesson" || got.SubagentOf != "parent-1" {
		t.Fatalf("metadata = %+v", got)
	}
	if got.InputTokens != 1000 || got.OutputTokens != 500 || got.CachedInputTokens != 200 {
		t.Fatalf("usage = %+v", got)
	}
}
