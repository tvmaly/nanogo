package graph

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Checkpoint struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	SessionID  string    `json:"session_id"`
	Pattern    string    `json:"pattern"`
	Node       string    `json:"node"`
	Step       int       `json:"step"`
	StateRef   string    `json:"state_ref,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ResumeKind string    `json:"resume_kind"`
}

type CheckpointStore struct {
	path string
}

func NewCheckpointStore(path string) *CheckpointStore {
	return &CheckpointStore{path: path}
}

func (s *CheckpointStore) Save(_ context.Context, rec Checkpoint) (string, error) {
	if rec.Version == 0 {
		rec.Version = 1
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("%s-%d", first(rec.RunID, "checkpoint"), rec.CreatedAt.UnixNano())
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return rec.ID, json.NewEncoder(f).Encode(rec)
}

func (s *CheckpointStore) Load(_ context.Context, id string) (Checkpoint, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return Checkpoint{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec Checkpoint
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return Checkpoint{}, err
		}
		if rec.ID == id {
			return rec, nil
		}
	}
	if err := sc.Err(); err != nil {
		return Checkpoint{}, err
	}
	return Checkpoint{}, fmt.Errorf("checkpoint %q not found", id)
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
