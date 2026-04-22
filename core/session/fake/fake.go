// Package fake provides a controllable Session and Store for tests.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
)

// Session is a fake in-memory Session.
type Session struct {
	mu     sync.Mutex
	id     string
	msgs   []llm.Message
	status session.Status
	turnID string
	waitCh chan string
}

func New(id string) *Session {
	return &Session{id: id, status: session.StatusActive}
}

func (s *Session) ID() string { return s.id }

func (s *Session) Messages() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Message, len(s.msgs))
	copy(out, s.msgs)
	return out
}

func (s *Session) Append(msg llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
}

func (s *Session) Save() error { return nil }

func (s *Session) SetWaiting(turnID string) <-chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan string, 1)
	s.status = session.StatusWaiting
	s.turnID = turnID
	s.waitCh = ch
	return ch
}

func (s *Session) Resume(turnID, answer string) {
	s.mu.Lock()
	ch := s.waitCh
	matching := s.turnID == turnID
	if matching {
		s.status = session.StatusActive
		s.waitCh = nil
		s.turnID = ""
	}
	s.mu.Unlock()
	if matching && ch != nil {
		ch <- answer
	}
}

func (s *Session) GetStatus() session.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Store is a fake in-memory Store.
type Store struct {
	mu       sync.Mutex
	sessions map[string]session.Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]session.Session)}
}

func (s *Store) Create(id string) (session.Session, error) {
	sess := New(id)
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Store) Load(id string) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return sess, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *Store) GC(_ context.Context, _ time.Duration) {}
