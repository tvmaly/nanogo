package context_guard_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/ext/harness/context_guard"
)

func TestContextGuardNotConfigured(t *testing.T) {
	t.Parallel()

	sensor := context_guard.New(context_guard.Config{})

	result := harness.ToolResult{
		Tool:   "read_file",
		Output: "content",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals when not configured, got %d", len(sigs))
	}
}

func TestContextGuardBelowThreshold(t *testing.T) {
	t.Parallel()

	sensor := context_guard.New(context_guard.Config{
		CapRatio:     0.8,
		ContextLimit: 1000,
		CurrentTokens: 500, // 50% usage
	})

	result := harness.ToolResult{
		Tool:   "read_file",
		Output: "content",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals below threshold, got %d", len(sigs))
	}
}

func TestContextGuardAboveThreshold(t *testing.T) {
	t.Parallel()

	sensor := context_guard.New(context_guard.Config{
		CapRatio:      0.8,
		ContextLimit:  1000,
		CurrentTokens: 900, // 90% usage (above 80% threshold)
	})

	result := harness.ToolResult{
		Tool:   "read_file",
		Output: "content",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)
	if len(sigs) == 0 {
		t.Error("expected signal above threshold, got 0")
	}
	if sigs[0].Severity != "warn" {
		t.Errorf("severity = %q, want warn", sigs[0].Severity)
	}
}
