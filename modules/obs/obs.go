// Package obs provides tiny pluggable logging and tracing hooks.
package obs

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Attrs   map[string]any
}

type Logger interface {
	Log(context.Context, Entry) error
}
type LoggerFunc func(context.Context, Entry) error

func (f LoggerFunc) Log(ctx context.Context, e Entry) error { return f(ctx, e) }

var (
	mu      sync.RWMutex
	loggers []Logger
	Trace   Tracer = noopTracer{}
)

func SetLoggers(ls ...Logger) { mu.Lock(); loggers = ls; mu.Unlock() }
func Reset()                  { SetLoggers(); Trace = noopTracer{} }

func Log(ctx context.Context, e Entry) error {
	mu.RLock()
	ls := append([]Logger(nil), loggers...)
	mu.RUnlock()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	for _, l := range ls {
		if err := l.Log(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

type textLogger struct {
	w  io.Writer
	mu sync.Mutex
}

func NewTextLogger(w io.Writer) Logger { return &textLogger{w: w} }
func (l *textLogger) Log(_ context.Context, e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := fmt.Fprintf(l.w, "%s %s\n", e.Level, e.Message)
	return err
}

type Span interface{ End(error) }
type Tracer interface {
	Start(context.Context, string) Span
}
type noopTracer struct{}

func (noopTracer) Start(context.Context, string) Span { return noopSpan{} }

type noopSpan struct{}

func (noopSpan) End(error) {}
