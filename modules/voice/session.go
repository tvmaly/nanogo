package voice

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tvmaly/nanogo/core/contracts"
)

var ErrSTTDisabled = errors.New("voice stt disabled")

type AgentGateway interface {
	SubmitVoiceText(ctx context.Context, req VoiceAgentRequest) (VoiceAgentResult, error)
}

type VoiceAgentRequest struct {
	SessionID string
	Text      string
	Locale    string
	Metadata  map[string]string
}

type VoiceAgentMessage struct {
	Role string
	Text string
}

type VoiceAgentResult struct {
	Text     string
	Messages []VoiceAgentMessage
	Metadata map[string]string
}

type AudioSink interface {
	WriteTTS(ctx context.Context, event contracts.TTSEvent) error
	Close(ctx context.Context) error
}

type VoiceState struct {
	STTEnabled              bool
	TTSEnabled              bool
	AutoSpeakFinalResponses bool
}

type VoiceStatePatch struct {
	STTEnabled              *bool
	TTSEnabled              *bool
	AutoSpeakFinalResponses *bool
}

type SessionConfig struct {
	SessionID string
	Locale    string
	STT       contracts.SpeechToText
	TTS       contracts.TextToSpeech
	Agent     AgentGateway
	Sink      AudioSink
	State     VoiceState
}

type VoiceSession struct {
	sessionID string
	locale    string
	stt       contracts.SpeechToText
	tts       contracts.TextToSpeech
	agent     AgentGateway
	sink      AudioSink

	mu        sync.Mutex
	state     VoiceState
	sttSess   contracts.STTSession
	activeTTS contracts.TTSStream
	closed    bool
}

func NewSession(cfg SessionConfig) (*VoiceSession, error) {
	if cfg.SessionID == "" {
		return nil, fmt.Errorf("voice session: session id is required")
	}
	if cfg.STT == nil {
		return nil, fmt.Errorf("voice session: stt provider is required")
	}
	if cfg.Agent == nil {
		return nil, fmt.Errorf("voice session: agent gateway is required")
	}
	if !cfg.State.STTEnabled && !cfg.State.TTSEnabled && !cfg.State.AutoSpeakFinalResponses {
		cfg.State.STTEnabled = true
	}
	s := &VoiceSession{
		sessionID: cfg.SessionID,
		locale:    cfg.Locale,
		stt:       cfg.STT,
		tts:       cfg.TTS,
		agent:     cfg.Agent,
		sink:      cfg.Sink,
		state:     cfg.State,
	}
	if s.state.STTEnabled {
		if err := s.openSTT(context.Background()); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *VoiceSession) State() VoiceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *VoiceSession) UpdateVoiceState(ctx context.Context, patch VoiceStatePatch) (VoiceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if patch.AutoSpeakFinalResponses != nil {
		s.state.AutoSpeakFinalResponses = *patch.AutoSpeakFinalResponses
	}
	if patch.TTSEnabled != nil {
		next := *patch.TTSEnabled
		if !next && s.activeTTS != nil {
			_ = s.activeTTS.Close(ctx)
			s.activeTTS = nil
		}
		s.state.TTSEnabled = next
	}
	if patch.STTEnabled != nil {
		next := *patch.STTEnabled
		if !next && s.sttSess != nil {
			_ = s.sttSess.Close(ctx)
			s.sttSess = nil
		}
		s.state.STTEnabled = next
		if next && s.sttSess == nil {
			if err := s.openSTTLocked(ctx); err != nil {
				return s.state, err
			}
		}
	}
	return s.state, nil
}

func (s *VoiceSession) ProcessAudio(ctx context.Context, frame contracts.AudioFrame) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("voice session %q: closed", s.sessionID)
	}
	if !s.state.STTEnabled {
		s.mu.Unlock()
		return ErrSTTDisabled
	}
	sttSess := s.sttSess
	s.mu.Unlock()

	if frame.SessionID == "" {
		frame.SessionID = s.sessionID
	}
	if err := sttSess.WriteAudio(ctx, frame); err != nil {
		return fmt.Errorf("voice session %q write audio: %w", s.sessionID, err)
	}
	return s.drainTranscripts(ctx, sttSess)
}

func (s *VoiceSession) Speak(ctx context.Context, text string, opts contracts.TTSOptions) error {
	s.mu.Lock()
	if !s.state.TTSEnabled {
		s.mu.Unlock()
		return nil
	}
	tts := s.tts
	sink := s.sink
	s.mu.Unlock()
	if tts == nil || sink == nil || text == "" {
		return nil
	}
	stream, err := tts.Synthesize(ctx, contracts.SynthesisRequest{SessionID: s.sessionID, Text: text, Options: opts})
	if err != nil {
		return fmt.Errorf("voice session %q synthesize: %w", s.sessionID, err)
	}
	s.mu.Lock()
	s.activeTTS = stream
	s.mu.Unlock()
	defer func() {
		_ = stream.Close(ctx)
		s.mu.Lock()
		if s.activeTTS == stream {
			s.activeTTS = nil
		}
		s.mu.Unlock()
	}()
	for event := range stream.Events() {
		if err := sink.WriteTTS(ctx, event); err != nil {
			return fmt.Errorf("voice session %q write tts: %w", s.sessionID, err)
		}
		if event.Kind == contracts.TTSEventDone {
			return nil
		}
		if event.Kind == contracts.TTSEventError {
			return fmt.Errorf("voice session %q tts error: %s", s.sessionID, event.Error)
		}
	}
	return nil
}

func (s *VoiceSession) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	if s.sttSess != nil {
		err = errors.Join(err, s.sttSess.Close(ctx))
		s.sttSess = nil
	}
	if s.activeTTS != nil {
		err = errors.Join(err, s.activeTTS.Close(ctx))
		s.activeTTS = nil
	}
	if s.sink != nil {
		err = errors.Join(err, s.sink.Close(ctx))
	}
	return err
}

func (s *VoiceSession) openSTT(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openSTTLocked(ctx)
}

func (s *VoiceSession) openSTTLocked(ctx context.Context) error {
	sttSess, err := s.stt.OpenSTTSession(ctx, contracts.STTOptions{SessionID: s.sessionID, Locale: s.locale, InterimResults: true})
	if err != nil {
		return fmt.Errorf("voice session %q open stt: %w", s.sessionID, err)
	}
	s.sttSess = sttSess
	return nil
}

func (s *VoiceSession) drainTranscripts(ctx context.Context, sttSess contracts.STTSession) error {
	for {
		select {
		case event, ok := <-sttSess.Events():
			if !ok {
				return nil
			}
			if event.Kind != contracts.TranscriptFinal {
				continue
			}
			if event.SessionID == "" {
				event.SessionID = s.sessionID
			}
			result, err := s.agent.SubmitVoiceText(ctx, VoiceAgentRequest{
				SessionID: event.SessionID,
				Text:      event.Text,
				Locale:    event.Locale,
				Metadata:  cloneMap(event.Metadata),
			})
			if err != nil {
				return fmt.Errorf("voice session %q submit transcript: %w", s.sessionID, err)
			}
			state := s.State()
			if state.TTSEnabled && state.AutoSpeakFinalResponses {
				if err := s.Speak(ctx, result.Text, contracts.TTSOptions{}); err != nil {
					return err
				}
			}
		default:
			return nil
		}
	}
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
