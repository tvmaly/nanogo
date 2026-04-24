// Package fake provides test doubles for harness interfaces.
package fake

import (
	"context"

	"github.com/tvmaly/nanogo/core/harness"
)

// Sensor is a fake Sensor for testing.
type Sensor struct {
	NameVal string
	Signals []harness.Signal
}

func (s *Sensor) Name() string { return s.NameVal }

func (s *Sensor) Observe(_ context.Context, _ harness.ToolResult) []harness.Signal {
	return s.Signals
}

// NewSensor constructs a Sensor with a given name and signals.
func NewSensor(name string, signals ...harness.Signal) *Sensor {
	return &Sensor{NameVal: name, Signals: signals}
}

// Guide is a fake Guide for testing.
type Guide struct {
	NameVal string
	Text    string
	Err     error
}

func (g *Guide) Name() string { return g.NameVal }

func (g *Guide) Inject(_ context.Context) (string, error) {
	return g.Text, g.Err
}

// NewGuide constructs a Guide with a given name and text.
func NewGuide(name, text string) *Guide {
	return &Guide{NameVal: name, Text: text}
}
