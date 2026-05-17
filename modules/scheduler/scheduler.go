// Package scheduler defines the Scheduler interface and registry.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Job struct {
	ID   string
	Spec string
	Next string
}

type Scheduler interface {
	Schedule(id string, spec string, fn func(context.Context)) error
	Remove(id string) error
	List() []Job
	Start(ctx context.Context) error
}

type Factory func(cfg json.RawMessage) (Scheduler, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

func Register(name string, f Factory) { mu.Lock(); factories[name] = f; mu.Unlock() }

func Build(name string, cfg json.RawMessage) (Scheduler, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("scheduler: unknown driver %q", name)
	}
	return f(cfg)
}
