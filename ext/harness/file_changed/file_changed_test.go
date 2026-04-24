package file_changed_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/ext/harness/file_changed"
)

// TEST-6.6: file_changed sensor
func TestFileChanged(t *testing.T) {
	t.Parallel()

	// Create a temp directory
	tmpDir := t.TempDir()

	sensor := file_changed.New(file_changed.Config{
		Dirs: []string{tmpDir},
	})

	// Write a file (agent operation)
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Record as agent operation
	result := harness.ToolResult{
		Tool:   "write_file",
		Output: testFile,
		Err:    nil,
	}
	sigs := sensor.Observe(context.Background(), result)
	if len(sigs) > 0 {
		t.Errorf("expected 0 signals after agent write, got %d", len(sigs))
	}

	// Wait a bit for debounce window
	time.Sleep(150 * time.Millisecond)

	// External write (not tracked as agent operation)
	if err := os.WriteFile(testFile, []byte("external"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Now observe (with a different tool)
	result2 := harness.ToolResult{
		Tool:   "read_file",
		Output: testFile,
		Err:    nil,
	}
	sigs = sensor.Observe(context.Background(), result2)

	// Should detect the external change
	if len(sigs) == 0 {
		t.Error("expected signal for external file change, got 0")
	}
}
