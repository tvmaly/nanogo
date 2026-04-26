// Package router registers the "router" LLM driver, which dispatches to
// named sub-providers based on context keys set by the agent loop.
package router

import (
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/llm"
)

type providerEntry struct {
	Driver string          `json:"driver"`
	Config json.RawMessage `json:"config"`
}

type routerConfig struct {
	Providers map[string]providerEntry `json:"providers"`
	Rules     []llm.Rule               `json:"rules"`
	Fallback  string                   `json:"fallback"`
}

func init() {
	llm.Register("router", func(cfg json.RawMessage) (llm.Provider, error) {
		var rc routerConfig
		if err := json.Unmarshal(cfg, &rc); err != nil {
			return nil, fmt.Errorf("router config: %w", err)
		}
		pp := make(map[string]llm.Provider, len(rc.Providers))
		for k, v := range rc.Providers {
			p, err := llm.Build(v.Driver, v.Config)
			if err != nil {
				return nil, fmt.Errorf("router provider %q: %w", k, err)
			}
			pp[k] = p
		}
		r := &llm.Router{Providers: pp, Rules: rc.Rules, Fallback: rc.Fallback}
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("router validate: %w", err)
		}
		return r, nil
	})
}
