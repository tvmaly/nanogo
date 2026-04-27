// Package file writes events as JSONL.
package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

type Config struct {
	Path string `json:"path"`
}

type Writer struct {
	mu sync.Mutex
	f  *os.File
}

func New(cfg Config) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f}, nil
}

func (w *Writer) Close() error { return w.f.Close() }

func (w *Writer) Record(_ context.Context, e event.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	rec := map[string]any{"time": e.At.Format(time.RFC3339Nano), "kind": e.Kind, "session": e.Session, "turn": e.Turn, "payload": e.Payload}
	return json.NewEncoder(w.f).Encode(rec)
}
