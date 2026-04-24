// Package harness provides sensor and guide interfaces for tool feedback and agent guidance.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolResult describes the outcome of a tool execution.
type ToolResult struct {
	Tool   string
	Args   json.RawMessage
	Output string
	Err    error
}

// Signal represents feedback from a sensor about a tool result.
type Signal struct {
	Severity string         // "error" | "warn" | "info"
	Message  string
	Fix      string
	Attrs    map[string]any
	Binding  bool // default false; when true, signal is binding (authoritative)
}

// Sensor observes tool results and emits feedback signals.
type Sensor interface {
	Name() string
	Observe(ctx context.Context, result ToolResult) []Signal
}

// Guide injects context before an LLM call.
type Guide interface {
	Name() string
	Inject(ctx context.Context) (string, error)
}

// SensorFactory constructs a Sensor from config.
type SensorFactory func(cfg json.RawMessage) (Sensor, error)

// GuideFactory constructs a Guide from config.
type GuideFactory func(cfg json.RawMessage) (Guide, error)

var (
	mu           sync.RWMutex
	sensors      = make(map[string]SensorFactory)
	guides       = make(map[string]GuideFactory)
	sensorOrder  []string // tracks registration order
	guideOrder   []string
)

// RegisterSensor registers a sensor factory.
func RegisterSensor(name string, f SensorFactory) {
	mu.Lock()
	defer mu.Unlock()
	sensors[name] = f
	sensorOrder = append(sensorOrder, name)
}

// RegisterGuide registers a guide factory.
func RegisterGuide(name string, f GuideFactory) {
	mu.Lock()
	defer mu.Unlock()
	guides[name] = f
	guideOrder = append(guideOrder, name)
}

// BuildSensor builds a sensor by name.
func BuildSensor(name string, cfg json.RawMessage) (Sensor, error) {
	mu.RLock()
	f, ok := sensors[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sensor %q not registered", name)
	}
	return f(cfg)
}

// BuildGuide builds a guide by name.
func BuildGuide(name string, cfg json.RawMessage) (Guide, error) {
	mu.RLock()
	f, ok := guides[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("guide %q not registered", name)
	}
	return f(cfg)
}

// AllSensorNames returns all registered sensor names in registration order.
func AllSensorNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(sensorOrder))
	copy(out, sensorOrder)
	return out
}

// AllGuideNames returns all registered guide names in registration order.
func AllGuideNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(guideOrder))
	copy(out, guideOrder)
	return out
}
