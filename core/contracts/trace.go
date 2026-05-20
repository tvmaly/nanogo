package contracts

import (
	"context"
	"time"
)

type TraceSink interface {
	EmitTrace(ctx context.Context, event TraceEvent) error
}

type TraceEvent struct {
	Version   int
	RunID     string
	SessionID string
	Pattern   string
	Node      string
	Step      int
	Kind      string
	Status    string
	Message   string
	Data      map[string]any
	CreatedAt time.Time
}
