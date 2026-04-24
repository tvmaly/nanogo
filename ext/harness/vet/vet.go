// Package vet implements a Sensor that runs `go vet` after write_file calls to .go files.
package vet

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/tvmaly/nanogo/core/harness"
)

// Config configures the vet sensor.
type Config struct {
	Enabled bool   `json:"enabled"`
	WorkDir string `json:"work_dir"` // if empty, uses file's directory
}

// Sensor checks for vet issues after write_file operations.
type Sensor struct {
	cfg Config
}

// New constructs a Sensor.
func New(cfg Config) *Sensor {
	return &Sensor{cfg: cfg}
}

func (s *Sensor) Name() string { return "vet" }

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
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}

	// Run go vet
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = workDir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		// Vet passed
		return nil
	}

	// Vet found issues
	output := out.String()
	return []harness.Signal{
		{
			Severity: "error",
			Message:  output,
			Fix:      "Fix the vet issues and try again.",
			Binding:  false,
		},
	}
}

func init() {
	harness.RegisterSensor("vet", func(cfg json.RawMessage) (harness.Sensor, error) {
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
