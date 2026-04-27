package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var ErrUnknownDriver = errors.New("unknown llm driver")

type Provider interface {
	Chat(ctx context.Context, req Request) (<-chan Chunk, error)
}

type Request struct {
	Model    string
	Messages []Message
	Tools    []ToolSchema
	Stream   bool
}

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type Chunk struct {
	TextDelta    string
	ToolCall     *ToolCall
	FinishReason string
	Usage        *Usage
	Err          error
}

type Usage struct {
	InputTokens, OutputTokens, CachedInputTokens int
	ServerToolUse                                map[string]int
}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type ToolSchema = json.RawMessage
type Factory func(cfg json.RawMessage) (Provider, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = f
}

func Build(name string, cfg json.RawMessage) (Provider, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDriver, name)
	}
	return f(cfg)
}

type ctxKey int

const (
	CtxKeySource ctxKey = iota
	CtxKeySkill
	CtxKeySubagent
)

type Rule struct {
	When, Route, Model string
}

type Router struct {
	Providers map[string]Provider
	Rules     []Rule
	Fallback  string
}

func (r *Router) Validate() error {
	for _, rule := range r.Rules {
		if _, ok := r.Providers[rule.Route]; rule.Route != "" && !ok {
			return fmt.Errorf("route %q not in providers map", rule.Route)
		}
	}
	if r.Fallback != "" {
		if _, ok := r.Providers[r.Fallback]; !ok {
			return fmt.Errorf("route %q not in providers map", r.Fallback)
		}
	}
	return nil
}

func (r *Router) Chat(ctx context.Context, req Request) (<-chan Chunk, error) {
	source, _ := ctx.Value(CtxKeySource).(string)
	skill, _ := ctx.Value(CtxKeySkill).(string)
	subagent, _ := ctx.Value(CtxKeySubagent).(bool)
	for _, rule := range r.Rules {
		if !matchRule(rule.When, source, skill, subagent) {
			continue
		}
		p, ok := r.Providers[rule.Route]
		if !ok {
			return nil, fmt.Errorf("route %q not found", rule.Route)
		}
		if rule.Model != "" {
			req.Model = rule.Model
		}
		return p.Chat(ctx, req)
	}
	if r.Fallback != "" {
		p, ok := r.Providers[r.Fallback]
		if !ok {
			return nil, fmt.Errorf("fallback route %q not found", r.Fallback)
		}
		return p.Chat(ctx, req)
	}
	return nil, errors.New("no matching rule and no fallback configured")
}

func matchRule(when, source, skill string, subagent bool) bool {
	switch {
	case when == "default":
		return true
	case len(when) > 7 && when[:7] == "source=":
		return source == when[7:]
	case len(when) > 6 && when[:6] == "skill=":
		return skill == when[6:]
	case when == "subagent=true":
		return subagent
	case when == "subagent=false":
		return !subagent
	}
	return false
}
