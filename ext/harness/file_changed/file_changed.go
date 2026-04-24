// Package file_changed implements a Sensor that detects external file changes.
package file_changed

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/tvmaly/nanogo/core/harness"
)

// Config configures the file_changed sensor.
type Config struct {
	Dirs []string `json:"dirs"` // directories to watch
}

// Sensor detects external file changes.
type Sensor struct {
	cfg    Config
	lastOp map[string]time.Time // track recent writes by the agent
}

// New constructs a Sensor.
func New(cfg Config) *Sensor {
	return &Sensor{
		cfg:    cfg,
		lastOp: make(map[string]time.Time),
	}
}

func (s *Sensor) Name() string { return "file_changed" }

func (s *Sensor) Observe(ctx context.Context, result harness.ToolResult) []harness.Signal {
	// If tool is write_file, record the timestamp
	if result.Tool == "write_file" {
		now := time.Now()
		s.lastOp[result.Output] = now
		return nil
	}

	// Check for external changes in watched directories
	var changed []string
	debounceWindow := 100 * time.Millisecond

	for _, dir := range s.cfg.Dirs {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			fi, _ := os.Stat(path)
			if fi == nil {
				return nil
			}
			modTime := fi.ModTime()
			lastOpTime, ok := s.lastOp[path]
			// If modified after last operation and not by us, it's external
			if !ok && time.Since(modTime) < debounceWindow {
				changed = append(changed, path)
			} else if ok && modTime.After(lastOpTime.Add(debounceWindow)) {
				// External change after our write
				changed = append(changed, path)
			}
			return nil
		}); err != nil {
			continue
		}
	}

	if len(changed) == 0 {
		return nil
	}

	return []harness.Signal{
		{
			Severity: "warn",
			Message:  "External file changes detected: " + formatChanged(changed),
			Fix:      "Review the changes and reconcile if needed.",
			Binding:  false,
		},
	}
}

func formatChanged(files []string) string {
	result := ""
	for i, f := range files {
		if i > 0 {
			result += ", "
		}
		if i >= 5 {
			result += "... and " + string(rune(len(files)-5)) + " more"
			break
		}
		result += filepath.Base(f)
	}
	return result
}

func init() {
	harness.RegisterSensor("file_changed", func(cfg json.RawMessage) (harness.Sensor, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return New(c), nil
	})
}
