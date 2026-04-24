package gotest_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/ext/harness/gotest"
)

// TEST-6.4: gotest sensor - clean test run (non-write_file should return no signals)
func TestCleanTestRun(t *testing.T) {
	t.Parallel()

	sensor := gotest.New(gotest.Config{})

	// Non-write_file tool should return no signals
	result := harness.ToolResult{
		Tool:   "read_file",
		Output: "content",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)

	// Should return no signals for non-write_file
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals for non-write_file, got %d", len(sigs))
	}
}

// TEST-6.5: gotest sensor - write_file on non-.go file
func TestGotestNonGoFile(t *testing.T) {
	t.Parallel()

	sensor := gotest.New(gotest.Config{})

	// write_file on non-.go file should return no signals
	result := harness.ToolResult{
		Tool:   "write_file",
		Output: "wrote readme.txt",
		Err:    nil,
	}

	sigs := sensor.Observe(context.Background(), result)

	// Should return no signals for non-.go files
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals for non-.go file, got %d", len(sigs))
	}
}
