// Package tools defines the Tool and Source interfaces and the builtin tool registry.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is a callable capability exposed to the LLM.
type Tool interface {
	Name() string
	Schema() json.RawMessage
	Call(ctx context.Context, args json.RawMessage) (string, error)
}

// TurnInfo carries per-turn context for dynamic tool sources.
type TurnInfo struct {
	Session     string
	Turn        int
	RecentTools []string
	RevealTool  func(name string)
}

// Source provides tools for a given turn.
type Source interface {
	Tools(ctx context.Context, turn TurnInfo) ([]Tool, error)
}

// SourceFactory constructs a Source from raw config JSON.
type SourceFactory func(cfg json.RawMessage) (Source, error)

// Runner is the interface the spawn tool uses to run a subagent.
// Implemented by core/agent and injected at wire time.
type Runner interface {
	RunSubagent(ctx context.Context, opts SubagentOpts) (string, error)
}

// SubagentOpts configures a spawned subagent.
type SubagentOpts struct {
	ParentSession string
	Goal          string
	Role          string   // subagent skill name, optional
	Model         string   // model override, optional
	Tools         []string // allowlist; nil = inherit parent
}

var (
	mu       sync.RWMutex
	registry = map[string]SourceFactory{}
)

// Register associates a driver name with a SourceFactory.
func Register(name string, f SourceFactory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = f
}

// Build constructs a Source by driver name.
func Build(name string, cfg json.RawMessage) (Source, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown tool source driver: %q", name)
	}
	return f(cfg)
}
