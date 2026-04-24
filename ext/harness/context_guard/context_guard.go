// Package context_guard implements a Sensor that warns when context is near capacity.
package context_guard

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/harness"
)

// Config configures the context_guard sensor.
type Config struct {
	CapRatio        float64 `json:"cap_ratio"`        // default 0.8
	ContextLimit    int     `json:"context_limit"`    // max context tokens
	CurrentTokens   int     `json:"current_tokens"`   // current usage (for testing)
	EnabledForTests bool   `json:"enabled_for_tests"` // allow injection for tests
}

// Sensor warns when context usage exceeds threshold.
type Sensor struct {
	cfg Config
}

// New constructs a Sensor.
func New(cfg Config) *Sensor {
	if cfg.CapRatio <= 0 {
		cfg.CapRatio = 0.8
	}
	return &Sensor{cfg: cfg}
}

func (s *Sensor) Name() string { return "context_guard" }

func (s *Sensor) Observe(ctx context.Context, result harness.ToolResult) []harness.Signal {
	if s.cfg.ContextLimit == 0 {
		// Not configured
		return nil
	}

	threshold := int(float64(s.cfg.ContextLimit) * s.cfg.CapRatio)
	if s.cfg.CurrentTokens > threshold {
		pct := float64(s.cfg.CurrentTokens) / float64(s.cfg.ContextLimit) * 100
		return []harness.Signal{
			{
				Severity: "warn",
				Message:  fmt.Sprintf("Context usage at %.0f%% of limit. Consider consolidating.", pct),
				Fix:      "Consolidate memory or summarize prior turns.",
				Binding:  false,
			},
		}
	}

	return nil
}

func init() {
	harness.RegisterSensor("context_guard", func(cfg json.RawMessage) (harness.Sensor, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return New(c), nil
	})
}
