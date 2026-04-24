package vet_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/ext/harness/vet"
)

func TestCleanVet(t *testing.T) {
	t.Parallel()

	sensor := vet.New(vet.Config{})

	result := harness.ToolResult{
		Tool:   "write_file",
		Output: "wrote example.go",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)

	// Clean vet should return no signals
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals for clean vet, got %d", len(sigs))
	}
}

func TestVetFailure(t *testing.T) {
	t.Parallel()

	sensor := vet.New(vet.Config{
		WorkDir: "/nonexistent/dir",
	})

	result := harness.ToolResult{
		Tool:   "write_file",
		Output: "wrote test.go",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)

	if len(sigs) > 0 && sigs[0].Severity != "error" {
		t.Errorf("severity = %q, want error", sigs[0].Severity)
	}
}
