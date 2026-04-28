// Package otel provides a lightweight span exporter that ships spans as JSON
// to an OTLP-compatible HTTP endpoint.
package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

// Config holds the exporter configuration.
type Config struct {
	Endpoint string `json:"endpoint"` // HTTP base URL of the OTLP collector
}

type spanRecord struct {
	Name      string `json:"name"`
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	ParentID  string `json:"parent_id,omitempty"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time,omitempty"`
}

type spanKey struct{}

type spanCtx struct {
	record *spanRecord
}

// Tracer collects spans and exports them on Flush.
type Tracer struct {
	cfg    Config
	mu     sync.Mutex
	spans  []*spanRecord
	client *http.Client
}

// New creates a Tracer pointing at the given endpoint.
func New(cfg Config) (*Tracer, error) {
	return &Tracer{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second}}, nil
}

// StartSpan creates a child span under the parent context.
func (t *Tracer) StartSpan(ctx context.Context, name string) context.Context {
	traceID := newID()
	parentID := ""
	if p, ok := ctx.Value(spanKey{}).(*spanCtx); ok {
		traceID = p.record.TraceID
		parentID = p.record.SpanID
	}
	r := &spanRecord{
		Name:      name,
		TraceID:   traceID,
		SpanID:    newID(),
		ParentID:  parentID,
		StartTime: time.Now().UTC().Format(time.RFC3339Nano),
	}
	t.mu.Lock()
	t.spans = append(t.spans, r)
	t.mu.Unlock()
	return context.WithValue(ctx, spanKey{}, &spanCtx{record: r})
}

// EndSpan marks the span in context as finished.
func (t *Tracer) EndSpan(ctx context.Context) {
	if sc, ok := ctx.Value(spanKey{}).(*spanCtx); ok {
		t.mu.Lock()
		sc.record.EndTime = time.Now().UTC().Format(time.RFC3339Nano)
		t.mu.Unlock()
	}
}

// RecordEvent creates and immediately ends a span named after the event kind.
func (t *Tracer) RecordEvent(ctx context.Context, e event.Event) {
	child := t.StartSpan(ctx, string(e.Kind))
	t.EndSpan(child)
}

// Flush exports all collected spans to the configured endpoint as JSON.
func (t *Tracer) Flush(ctx context.Context) error {
	t.mu.Lock()
	spans := make([]*spanRecord, len(t.spans))
	copy(spans, t.spans)
	t.spans = t.spans[:0]
	t.mu.Unlock()

	if len(spans) == 0 {
		return nil
	}
	payload := map[string]any{"spans": spans}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Shutdown is a no-op for this lightweight exporter.
func (t *Tracer) Shutdown(_ context.Context) error { return nil }

var idMu sync.Mutex
var idCounter uint64

func newID() string {
	idMu.Lock()
	idCounter++
	v := idCounter
	idMu.Unlock()
	return fmt.Sprintf("%016x", v)
}
