package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/event"
)

// App is the interface transports use to drive the agent.
type App interface {
	Submit(ctx context.Context, session, message string) error
	Resume(ctx context.Context, session string, answer string) error
	TriggerSkill(ctx context.Context, name string, args map[string]any) error
}

// Transport is a pluggable input/output channel for the agent.
type Transport interface {
	Name() string
	Start(ctx context.Context, app App) error
	Stop(ctx context.Context) error
}

// Factory constructs a Transport.
type Factory func(cfg json.RawMessage, bus event.Bus, app App) (Transport, error)

var registry = map[string]Factory{}

// Register associates a driver name with a factory.
func Register(name string, f Factory) {
	registry[name] = f
}

// Build constructs a Transport by driver name.
func Build(name string, cfg json.RawMessage, bus event.Bus, app App) (Transport, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown transport driver: %q", name)
	}
	return f(cfg, bus, app)
}

// Registered returns the names of all registered transport drivers.
func Registered() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
