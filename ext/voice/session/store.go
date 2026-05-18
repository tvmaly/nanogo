package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

type Store struct {
	workspace  string
	persistRaw bool
}

func NewStore(workspace string) *Store {
	return NewStoreWithPolicy(workspace, false)
}

func NewStoreWithPolicy(workspace string, persistRaw bool) *Store {
	if workspace == "" {
		workspace = "."
	}
	return &Store{workspace: workspace, persistRaw: persistRaw}
}

func (s *Store) WriteEvent(sess *Session, event realtime.Event) error {
	dir := filepath.Join(s.workspace, "memory", "voice")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if event.SchemaVersion == "" {
		event.SchemaVersion = "voice.event.v1"
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := appendFile(filepath.Join(dir, sess.ID+"_events.jsonl"), b); err != nil {
		return err
	}
	if s.persistRaw && len(event.Raw) > 0 {
		raw, _ := json.Marshal(map[string]any{
			"schema_version": "voice.raw_provider_event.v1",
			"session_id":     sess.ID,
			"provider":       sess.Provider,
			"type":           event.Type,
			"raw":            json.RawMessage(event.Raw),
			"at":             event.At,
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
