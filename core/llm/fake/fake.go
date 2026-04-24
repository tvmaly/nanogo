// Package fake provides a controllable Provider for tests.
package fake

import (
	"context"

	"github.com/tvmaly/nanogo/core/llm"
)

// Provider replays a fixed sequence of Chunk slices, one per Chat call.
type Provider struct {
	Responses [][]llm.Chunk
	Calls     int
}

func New(responses ...[]llm.Chunk) *Provider {
	return &Provider{Responses: responses}
}

// FuncProvider calls a custom function for each Chat call.
type FuncProvider struct {
	Fn func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error)
}

// NewFunc creates a Provider backed by an arbitrary function.
func NewFunc(fn func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error)) *FuncProvider {
	return &FuncProvider{Fn: fn}
}

func (p *FuncProvider) Chat(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	return p.Fn(ctx, req)
}

func (p *Provider) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	idx := p.Calls
	p.Calls++
	var chunks []llm.Chunk
	if idx < len(p.Responses) {
		chunks = p.Responses[idx]
	} else if len(p.Responses) > 0 {
		chunks = p.Responses[len(p.Responses)-1]
	}
	ch := make(chan llm.Chunk, len(chunks)+1)
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}
