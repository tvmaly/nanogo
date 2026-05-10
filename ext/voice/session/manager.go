package session

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

type Config struct {
	Workspace     string
	Provider      realtime.ProviderAdapter
	ProviderCfg   realtime.ProviderConfig
	SessionUpdate realtime.Event
}

type Manager struct {
	mu       sync.Mutex
	cfg      Config
	store    *Store
	sessions map[string]*Session
	next     int
}

type Session struct {
	ID       string
	Provider string
	conn     realtime.RealtimeConn
	events   chan realtime.Event
	done     chan struct{}
}

func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:      cfg,
		store:    NewStore(cfg.Workspace),
		sessions: map[string]*Session{},
	}
}

func (m *Manager) Start(ctx context.Context) (*Session, error) {
	if m.cfg.Provider == nil {
		return nil, fmt.Errorf("voice session: provider is required")
	}
	conn, err := m.cfg.Provider.Connect(ctx, m.cfg.ProviderCfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.next++
	id := fmt.Sprintf("voice-%d", m.next)
	s := &Session{
		ID:       id,
		Provider: m.cfg.Provider.Name(),
		conn:     conn,
		events:   make(chan realtime.Event, 64),
		done:     make(chan struct{}),
	}
	m.sessions[id] = s
	m.mu.Unlock()

	update := m.cfg.SessionUpdate
	if update.Type == "" {
		update = realtime.Event{Type: realtime.EventSessionUpdate}
	}
	update.SessionID = id
	if err := conn.Send(ctx, update); err != nil {
		return nil, err
	}
	_ = m.store.WriteEvent(s, update)
	go m.receiveLoop(ctx, s)
	return s, nil
}

func (m *Manager) Events(sessionID string) (<-chan realtime.Event, error) {
	s, err := m.session(sessionID)
	if err != nil {
		return nil, err
	}
	return s.events, nil
}

func (m *Manager) TextSend(ctx context.Context, sessionID, text string) error {
	return m.send(ctx, sessionID, realtime.TextMessage(text))
}

func (m *Manager) ResponseCreate(ctx context.Context, sessionID string) error {
	return m.send(ctx, sessionID, realtime.Event{Type: realtime.EventResponseCreate})
}

func (m *Manager) AudioAppend(ctx context.Context, sessionID, audioBase64 string) error {
	if _, err := base64.StdEncoding.DecodeString(audioBase64); err != nil {
		return fmt.Errorf("voice_audio_append: invalid base64 PCM: %w", err)
	}
	return m.send(ctx, sessionID, realtime.Event{Type: realtime.EventInputAudioBufferAppend, AudioBase64: audioBase64})
}

func (m *Manager) AudioCommit(ctx context.Context, sessionID string) error {
	return m.send(ctx, sessionID, realtime.Event{Type: realtime.EventInputAudioBufferCommit})
}

func (m *Manager) AudioClear(ctx context.Context, sessionID string) error {
	return m.send(ctx, sessionID, realtime.Event{Type: realtime.EventInputAudioBufferClear})
}

func (m *Manager) Close(sessionID string) error {
	s, err := m.session(sessionID)
	if err != nil {
		return err
	}
	err = s.conn.Close()
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return err
}

func (m *Manager) send(ctx context.Context, sessionID string, event realtime.Event) error {
	s, err := m.session(sessionID)
	if err != nil {
		return err
	}
	event.SessionID = sessionID
	event.Provider = s.Provider
	event.At = time.Now().UTC()
	if err := s.conn.Send(ctx, event); err != nil {
		return err
	}
	return m.store.WriteEvent(s, event)
}

func (m *Manager) session(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil {
		return nil, fmt.Errorf("voice session %q not found", id)
	}
	return s, nil
}

func (m *Manager) receiveLoop(ctx context.Context, s *Session) {
	defer func() {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
	}()
	for {
		event, err := s.conn.Receive(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			event = realtime.Event{Type: realtime.EventError, Error: err.Error()}
		}
		event.SessionID = s.ID
		event.Provider = s.Provider
		event.At = time.Now().UTC()
		_ = m.store.WriteEvent(s, event)
		select {
		case s.events <- event:
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
		if event.Type == realtime.EventError || event.Type == realtime.EventResponseDone {
			if err != nil {
				return
			}
		}
	}
}
