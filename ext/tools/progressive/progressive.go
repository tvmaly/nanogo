// Package progressive implements a tool source with progressive disclosure.
package progressive

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Source holds a set of tools, some hidden until revealed.
type Source struct {
	mu      sync.RWMutex
	all     map[string]json.RawMessage
	visible map[string]bool
}

// NewSource creates an empty progressive Source. All registered tools start hidden.
func NewSource() *Source {
	return &Source{
		all:     make(map[string]json.RawMessage),
		visible: make(map[string]bool),
	}
}

// Register adds a tool schema under name. It starts hidden.
func (s *Source) Register(name string, schema json.RawMessage) {
	s.mu.Lock()
	s.all[name] = schema
	s.mu.Unlock()
}

// RevealTool makes the named tool visible in subsequent Tools calls.
// Returns an error if the tool has not been registered.
func (s *Source) RevealTool(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.all[name]; !ok {
		return fmt.Errorf("progressive: tool %q not registered", name)
	}
	s.visible[name] = true
	return nil
}

// Tools returns the schemas of all currently visible tools.
func (s *Source) Tools(_ context.Context) []json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []json.RawMessage
	for name, schema := range s.all {
		if s.visible[name] {
			out = append(out, schema)
		}
	}
	return out
}
