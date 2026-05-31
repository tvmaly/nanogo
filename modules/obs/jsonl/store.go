// Package jsonl stores agent-readable observation records as JSON lines.
package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tvmaly/nanogo/modules/obs"
)

type Config struct {
	Root             string            `json:"root"`
	Path             string            `json:"path"`
	FailurePolicy    obs.FailurePolicy `json:"failure_policy"`
	FlushOnError     bool              `json:"flush_on_error"`
	FlushOnRunFinish bool              `json:"flush_on_run_finish"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	f      *os.File
	w      *bufio.Writer
	closed bool
}

func New(cfg Config) (*Store, error) {
	path := cfg.Path
	if path == "" {
		path = "observations.jsonl"
	}
	if !filepath.IsAbs(path) {
		root := cfg.Root
		if root == "" {
			root = filepath.Join(".nanogo", "obs")
		}
		path = filepath.Join(root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("obs jsonl mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("obs jsonl open: %w", err)
	}
	return &Store{path: path, f: f, w: bufio.NewWriter(f)}, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Append(_ context.Context, rec obs.ObservationRecord) error {
	if err := rec.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return obs.ErrStoreClosed
	}
	if err := json.NewEncoder(s.w).Encode(rec); err != nil {
		return fmt.Errorf("obs jsonl append: %w", err)
	}
	return nil
}

func (s *Store) Query(context.Context, obs.QuerySpec) (obs.QueryResult, error) {
	return obs.QueryResult{}, obs.ErrQueryNotImplemented
}

func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return obs.ErrStoreClosed
	}
	if err := s.w.Flush(); err != nil {
		return fmt.Errorf("obs jsonl flush buffer: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		return fmt.Errorf("obs jsonl sync: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	var flushErr error
	if s.w != nil {
		flushErr = s.w.Flush()
	}
	closeErr := s.f.Close()
	s.closed = true
	if flushErr != nil {
		return fmt.Errorf("obs jsonl close flush: %w", flushErr)
	}
	if closeErr != nil {
		return fmt.Errorf("obs jsonl close: %w", closeErr)
	}
	return nil
}
