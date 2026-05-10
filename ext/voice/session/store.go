package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

type Store struct {
	workspace string
}

func NewStore(workspace string) *Store {
	if workspace == "" {
		workspace = "."
	}
	return &Store{workspace: workspace}
}

func (s *Store) WriteEvent(sess *Session, event realtime.Event) error {
	dir := filepath.Join(s.workspace, "memory", "voice")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := appendFile(filepath.Join(dir, sess.ID+"_events.jsonl"), b); err != nil {
		return err
	}
	if len(event.Raw) > 0 {
		raw, _ := json.Marshal(map[string]any{
			"session_id": sess.ID,
			"provider":   sess.Provider,
			"type":       event.Type,
			"raw":        json.RawMessage(event.Raw),
			"at":         event.At,
		})
		raw = append(raw, '\n')
		return appendFile(filepath.Join(dir, sess.ID+"_raw.jsonl"), raw)
	}
	return nil
}

func appendFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}
