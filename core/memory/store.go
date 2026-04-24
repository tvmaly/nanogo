package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HistoryEntry is a single record in memory/history.jsonl.
type HistoryEntry struct {
	At      time.Time `json:"at"`
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Cursor  bool      `json:"cursor,omitempty"`
}

// Store handles file I/O for the workspace memory layout.
type Store struct {
	root string // workspace root, e.g. ~/.nanogo/workspace
}

// NewStore creates a Store rooted at root. The directory is created if absent.
func NewStore(root string) (*Store, error) {
	dirs := []string{
		root,
		filepath.Join(root, "memory", "topics"),
		filepath.Join(root, "memory", "procedures"),
		filepath.Join(root, "memory", "archive"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{root: root}, nil
}

// Root returns the workspace root.
func (s *Store) Root() string { return s.root }

// ReadFile returns the content of a workspace-relative file, or "" if absent.
func (s *Store) ReadFile(rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.root, rel))
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

// WriteFile writes content to a workspace-relative path, creating parents.
func (s *Store) WriteFile(rel, content string) error {
	full := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// AppendHistory appends a HistoryEntry to memory/history.jsonl.
func (s *Store) AppendHistory(e HistoryEntry) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.root, "memory", "history.jsonl"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(string(line) + "\n")
	return err
}

// ReadHistory reads all entries from memory/history.jsonl.
func (s *Store) ReadHistory() ([]HistoryEntry, error) {
	data, err := s.ReadFile("memory/history.jsonl")
	if err != nil || data == "" {
		return nil, err
	}
	var entries []HistoryEntry
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if line == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// AppendJSONL appends a JSON line to any workspace-relative .jsonl file.
func (s *Store) AppendJSONL(rel string, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	full := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(string(line) + "\n")
	return err
}
