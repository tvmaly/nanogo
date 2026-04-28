// Package classifierrouter implements a two-stage LLM router that uses a cheap
// classifier model to label each request, then dispatches to the appropriate
// downstream provider based on that label.
package classifierrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tvmaly/nanogo/core/llm"
)

// Config holds the classifier router configuration.
type Config struct {
	Classifier   llm.Provider
	Routes       map[string]llm.Provider // label → provider
	DefaultRoute string
}

// Router is a two-stage LLM provider.
type Router struct {
	cfg   Config
	mu    sync.Mutex
	cache map[string]string // prompt fingerprint → label
}

type providerEntry struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
}

type buildConfig struct {
	Classifier providerEntry            `json:"classifier"`
	Routes     map[string]providerEntry `json:"routes"`
	Default    string                   `json:"default"`
}

func init() {
	llm.Register("classifier-router", func(raw json.RawMessage) (llm.Provider, error) {
		var cfg buildConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("classifier-router config: %w", err)
		}
		if cfg.Classifier.Driver == "" {
			return nil, fmt.Errorf("classifier-router config: classifier.driver is required")
		}
		classifier, err := llm.Build(cfg.Classifier.Driver, cfg.Classifier.Config)
		if err != nil {
			return nil, fmt.Errorf("classifier-router classifier: %w", err)
		}
		routes := make(map[string]llm.Provider, len(cfg.Routes))
		for label, entry := range cfg.Routes {
			if entry.Driver == "" {
				return nil, fmt.Errorf("classifier-router route %q: driver is required", label)
			}
			p, err := llm.Build(entry.Driver, entry.Config)
			if err != nil {
				return nil, fmt.Errorf("classifier-router route %q: %w", label, err)
			}
			routes[label] = p
		}
		if cfg.Default == "" {
			cfg.Default = "default"
		}
		if _, ok := routes[cfg.Default]; !ok {
			return nil, fmt.Errorf("classifier-router default route %q not configured", cfg.Default)
		}
		return New(Config{Classifier: classifier, Routes: routes, DefaultRoute: cfg.Default}), nil
	})
}

// New creates a Router with the given config.
func New(cfg Config) *Router {
	return &Router{cfg: cfg, cache: make(map[string]string)}
}

// Chat classifies the request then dispatches to the appropriate provider.
func (r *Router) Chat(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	key := cacheKey(req)

	r.mu.Lock()
	label, hit := r.cache[key]
	r.mu.Unlock()

	if !hit {
		label = r.classify(ctx, req)
		r.mu.Lock()
		r.cache[key] = label
		r.mu.Unlock()
	}

	provider, ok := r.cfg.Routes[label]
	if !ok {
		provider = r.cfg.Routes[r.cfg.DefaultRoute]
	}
	return provider.Chat(ctx, req)
}

// classify calls the classifier and returns its trimmed text output as the label.
func (r *Router) classify(ctx context.Context, req llm.Request) string {
	ch, err := r.cfg.Classifier.Chat(ctx, req)
	if err != nil {
		return r.cfg.DefaultRoute
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.TextDelta)
	}
	label := strings.TrimSpace(sb.String())
	if _, ok := r.cfg.Routes[label]; !ok {
		return r.cfg.DefaultRoute
	}
	return label
}

// cacheKey builds a fingerprint from the last user message content.
func cacheKey(req llm.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return ""
}
