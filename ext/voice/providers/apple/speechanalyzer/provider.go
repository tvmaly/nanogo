package speechanalyzer

import (
	"context"
	"fmt"
	"runtime"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/voice/providers/apple/helper"
)

const (
	providerName = "apple_speechanalyzer"
	envHelper    = "NANOGO_APPLE_SPEECHANALYZER_HELPER"
	binaryName   = "apple-speechanalyzer-helper"
)

type Provider struct {
	helperPath string
}

func New() *Provider {
	return &Provider{}
}

func NewWithHelper(path string) *Provider {
	return &Provider{helperPath: path}
}

func (p *Provider) Capabilities(context.Context) (contracts.STTCapabilities, error) {
	if runtime.GOOS != "darwin" {
		return contracts.STTCapabilities{}, fmt.Errorf("%w: apple speechanalyzer requires macOS", helper.ErrUnavailable)
	}
	if _, err := p.resolveHelper(); err != nil {
		return contracts.STTCapabilities{}, err
	}
	return contracts.STTCapabilities{
		Provider:        providerName,
		Local:           true,
		Streaming:       true,
		OfflineFiles:    false,
		Locales:         []string{"en-US"},
		SupportsPartial: true,
		SupportsTiming:  true,
		InputFormats: []contracts.AudioFormat{{
			Encoding:     contracts.AudioEncodingPCM16,
			SampleRateHz: 24000,
			Channels:     1,
		}},
	}, nil
}

func (p *Provider) OpenSTTSession(ctx context.Context, opts contracts.STTOptions) (contracts.STTSession, error) {
	if runtime.GOOS != "darwin" && p.helperPath == "" {
		return nil, fmt.Errorf("%w: apple speechanalyzer requires macOS", helper.ErrUnavailable)
	}
	path, err := p.resolveHelper()
	if err != nil {
		return nil, err
	}
	proc, err := (helper.Client{Path: path}).Start(ctx)
	if err != nil {
		return nil, err
	}
	if err := proc.Send(helper.STTStartRequest(opts)); err != nil {
		_ = proc.Close(ctx)
		return nil, err
	}
	session := &sttSession{sessionID: opts.SessionID, proc: proc, events: make(chan contracts.TranscriptEvent, 16)}
	go session.drain()
	return session, nil
}

func (p *Provider) resolveHelper() (string, error) {
	if p.helperPath != "" {
		return p.helperPath, nil
	}
	return helper.ResolveBinary(envHelper, binaryName)
}

type sttSession struct {
	sessionID string
	proc      *helper.Process
	events    chan contracts.TranscriptEvent
}

func (s *sttSession) WriteAudio(_ context.Context, frame contracts.AudioFrame) error {
	if frame.SessionID == "" {
		frame.SessionID = s.sessionID
	}
	return s.proc.Send(helper.STTAudioRequest(frame))
}

func (s *sttSession) Events() <-chan contracts.TranscriptEvent { return s.events }

func (s *sttSession) Close(ctx context.Context) error {
	_ = s.proc.Send(helper.Request{Type: "close", SessionID: s.sessionID})
	return s.proc.Close(ctx)
}

func (s *sttSession) drain() {
	defer close(s.events)
	for raw := range s.proc.Events() {
		event, err := helper.TranscriptEvent(raw)
		if err != nil {
			s.events <- contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Metadata: map[string]string{"error": err.Error()}}
			continue
		}
		if event.SessionID == "" {
			event.SessionID = s.sessionID
		}
		s.events <- event
	}
}
