// Package session provides JSONL-backed conversation sessions with resumable checkpoints.
package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/llm"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusWaiting Status = "waiting_for_input"
	StatusFailed  Status = "failed"
)

type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Session interface {
	ID() string
	Messages() []llm.Message
	Append(msg llm.Message)
	Save() error
	SetWaiting(turnID string) <-chan string
	Resume(turnID, answer string)
	GetStatus() Status
}

type Store interface {
	Create(id string) (Session, error)
	Load(id string) (Session, error)
	Delete(id string) error
	GC(ctx context.Context, ttl time.Duration)
}

type msgRecord struct {
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	At         time.Time `json:"at"`
}

type metaRecord struct {
	Status  Status    `json:"status"`
	SavedAt time.Time `json:"saved_at"`
}

type impl struct {
	mu              sync.Mutex
	id, path, metaP string
	msgs            []llm.Message
	status          Status
	turnID          string
	waitCh          chan string
	savedAt         time.Time
	clock           Clock
}

func (s *impl) ID() string { return s.id }

func (s *impl) Messages() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Message, len(s.msgs))
	copy(out, s.msgs)
	return out
}

func (s *impl) Append(msg llm.Message) {
	s.mu.Lock()
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()
}

func (s *impl) SetWaiting(turnID string) <-chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan string, 1)
	s.status, s.turnID, s.waitCh = StatusWaiting, turnID, ch
	return ch
}

func (s *impl) Resume(turnID, answer string) {
	s.mu.Lock()
	ch, ok := s.waitCh, s.turnID == turnID
	if ok {
		s.status, s.waitCh, s.turnID = StatusActive, nil, ""
	}
	s.mu.Unlock()
	if ok && ch != nil {
		ch <- answer
	}
}

func (s *impl) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *impl) Save() error {
	s.mu.Lock()
	msgs, status, now := append([]llm.Message(nil), s.msgs...), s.status, s.clock.Now()
	s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("session save %s: %w", s.id, err)
	}
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(msgRecord{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, At: now}); err != nil {
			f.Close(); return err
		}
	}
	f.Close()
	raw, _ := json.Marshal(metaRecord{Status: status, SavedAt: now})
	return os.WriteFile(s.metaP, raw, 0644)
}

// store implements Store with JSONL files.
type store struct {
	dir   string
	clock Clock
}

// NewStore creates a Store rooted at dir. If clock is nil, uses real time.
func NewStore(dir string, clock Clock) Store {
	if clock == nil {
		clock = realClock{}
	}
	return &store{dir: dir, clock: clock}
}

func (s *store) msgPath(id string) string  { return filepath.Join(s.dir, id+".jsonl") }
func (s *store) metaPath(id string) string { return filepath.Join(s.dir, id+".meta.json") }

func (s *store) Create(id string) (Session, error) {
	return &impl{id: id, path: s.msgPath(id), metaP: s.metaPath(id),
		status: StatusActive, clock: s.clock, savedAt: s.clock.Now()}, nil
}

func (s *store) Load(id string) (Session, error) {
	f, err := os.Open(s.msgPath(id))
	if err != nil {
		return nil, fmt.Errorf("session load %s: %w", id, err)
	}
	defer f.Close()
	var msgs []llm.Message
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r msgRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("session load %s: %w", id, err)
		}
		msgs = append(msgs, llm.Message{Role: r.Role, Content: r.Content, ToolCallID: r.ToolCallID})
	}
	m := metaRecord{Status: StatusActive, SavedAt: s.clock.Now()}
	if raw, err := os.ReadFile(s.metaPath(id)); err == nil {
		_ = json.Unmarshal(raw, &m)
	}
	return &impl{id: id, path: s.msgPath(id), metaP: s.metaPath(id),
		msgs: msgs, status: m.Status, clock: s.clock, savedAt: m.SavedAt}, nil
}

func (s *store) Delete(id string) error { _ = os.Remove(s.metaPath(id)); return os.Remove(s.msgPath(id)) }

func (s *store) GC(ctx context.Context, ttl time.Duration) {
	entries, _ := os.ReadDir(s.dir)
	cutoff := s.clock.Now().Add(-ttl)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var m metaRecord
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if m.Status == StatusWaiting && m.SavedAt.Before(cutoff) {
			id := strings.TrimSuffix(e.Name(), ".meta.json")
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
			_ = os.Remove(s.msgPath(id))
		}
	}
}
