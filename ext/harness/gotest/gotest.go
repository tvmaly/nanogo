// Package gotest implements a Sensor that runs `go test` after write_file calls to .go files.
package gotest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/tvmaly/nanogo/core/harness"
)

// Config configures the gotest sensor.
type Config struct {
	Enabled bool   `json:"enabled"`
	WorkDir string `json:"work_dir"` // if empty, uses file's directory
}

// Sensor checks for test failures after write_file operations.
type Sensor struct {
	cfg Config
}

// New constructs a Sensor.
func New(cfg Config) *Sensor {
	return &Sensor{cfg: cfg}
}

func (s *Sensor) Name() string { return "gotest" }

func (s *Sensor) Observe(ctx context.Context, result harness.ToolResult) []harness.Signal {
	// Only run if tool is write_file
	if result.Tool != "write_file" {
		return nil
	}

	// Only run if output contains .go
	if !strings.Contains(result.Output, ".go") {
		return nil
	}

	// Determine working directory
	workDir := s.cfg.WorkDir
	if workDir == "" {
		// Try to extract from result output
		// For now, use current directory
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}

	// Run go test
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = workDir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		// Tests passed
		return nil
	}

	// Tests failed
	output := out.String()
	return []harness.Signal{
		{
			Severity: "error",
			Message:  output,
			Fix:      "Fix the failing test(s) and try again.",
			Binding:  false, // advisory, not binding
		},
	}
}

func init() {
	harness.RegisterSensor("gotest", func(cfg json.RawMessage) (harness.Sensor, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		c.Enabled = true
		return New(c), nil
	})
}
