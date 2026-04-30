// Package profile stores child adaptive preferences with parent approval state.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type State string

const (
	Pending  State = "pending"
	Approved State = "approved"
	Edited   State = "edited"
	Rejected State = "rejected"
)

type Profile struct {
	ChildID     string            `json:"child_id"`
	Preferences map[string]string `json:"preferences,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Change struct {
	ID        string    `json:"id"`
	ChildID   string    `json:"child_id"`
	Field     string    `json:"field"`
	Proposed  string    `json:"proposed"`
	Edited    string    `json:"edited,omitempty"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(root string) (*Store, error) {
	dir := filepath.Join(root, "memory", "adaptive", "profiles")
	return &Store{dir: dir}, os.MkdirAll(dir, 0755)
}

func (s *Store) Read(_ context.Context, childID string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.readProfile(childID)
	if os.IsNotExist(err) {
		return Profile{ChildID: childID, Preferences: map[string]string{}}, nil
	}
	return p, err
}

func (s *Store) Propose(_ context.Context, ch Change) (Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	ch.ID = fmt.Sprintf("%s-%d", ch.ChildID, now.UnixNano())
	ch.State = Pending
	ch.CreatedAt = now
	ch.UpdatedAt = now
	return ch, appendJSON(filepath.Join(s.dir, "changes.jsonl"), ch)
}

func (s *Store) Resolve(_ context.Context, id string, state State, edited string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changes, err := s.readChanges("")
	if err != nil {
		return err
	}
	for i := range changes {
		if changes[i].ID != id {
			continue
		}
		changes[i].State = state
		changes[i].Edited = edited
		changes[i].UpdatedAt = time.Now().UTC()
		if state == Approved || state == Edited {
			value := changes[i].Proposed
			if state == Edited {
				value = edited
			}
			p, err := s.readProfile(changes[i].ChildID)
			if os.IsNotExist(err) {
				p = Profile{ChildID: changes[i].ChildID, Preferences: map[string]string{}}
			} else if err != nil {
				return err
			}
			p.Preferences[changes[i].Field] = value
			p.UpdatedAt = changes[i].UpdatedAt
			if err := s.writeProfile(p); err != nil {
				return err
			}
		}
		return s.writeChanges(changes)
	}
	return fmt.Errorf("profile change %q not found", id)
}

func (s *Store) Changes(_ context.Context, childID string) ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readChanges(childID)
}

func (s *Store) readProfile(childID string) (Profile, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, childID+".json"))
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	return p, json.Unmarshal(data, &p)
}

func (s *Store) writeProfile(p Profile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, p.ChildID+".json"), data, 0644)
}

func (s *Store) readChanges(childID string) ([]Change, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "changes.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Change
	for _, line := range splitLines(data) {
		var ch Change
		if err := json.Unmarshal(line, &ch); err != nil {
			return nil, err
		}
		if childID == "" || ch.ChildID == childID {
			out = append(out, ch)
		}
	}
	return out, nil
}

func (s *Store) writeChanges(changes []Change) error {
	tmp := filepath.Join(s.dir, "changes.jsonl")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, ch := range changes {
		if err := enc.Encode(ch); err != nil {
			return err
		}
	}
	return nil
}

func appendJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				out = append(out, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
