package router_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
	_ "github.com/tvmaly/nanogo/ext/llm/router"
)

func TestRouterFactory(t *testing.T) {
	t.Parallel()
	llm.Register("fake-ra", func(_ json.RawMessage) (llm.Provider, error) { return &fp{}, nil })
	llm.Register("fake-rb", func(_ json.RawMessage) (llm.Provider, error) { return &fp{}, nil })

	cfg := json.RawMessage(`{
		"providers": {
			"cheap":    {"driver":"fake-ra","config":{}},
			"standard": {"driver":"fake-rb","config":{}}
		},
		"rules": [
			{"when":"source=heartbeat","route":"cheap"},
			{"when":"default","route":"standard"}
		],
		"fallback": "standard"
	}`)

	p, err := llm.Build("router", cfg)
	if err != nil {
		t.Fatalf("Build router: %v", err)
	}

	ctx := context.WithValue(context.Background(), llm.CtxKeySource, "heartbeat")
	ch, err := p.Chat(ctx, llm.Request{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	for range ch {}
}

type fp struct{}

func (f *fp) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, 1)
	ch <- llm.Chunk{TextDelta: "ok", FinishReason: "stop"}
	close(ch)
	return ch, nil
}
