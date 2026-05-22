package avspeech

import (
	"context"
	"fmt"
	"runtime"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/voice/providers/apple/helper"
)

const (
	providerName = "apple_avspeech"
	envHelper    = "NANOGO_APPLE_AVSPEECH_HELPER"
	binaryName   = "apple-avspeech-helper"
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

func (p *Provider) Capabilities(context.Context) (contracts.TTSCapabilities, error) {
	if runtime.GOOS != "darwin" {
		return contracts.TTSCapabilities{}, fmt.Errorf("%w: apple avspeech requires macOS", helper.ErrUnavailable)
	}
	if _, err := p.resolveHelper(); err != nil {
		return contracts.TTSCapabilities{}, err
	}
	return contracts.TTSCapabilities{
		Provider:  providerName,
		Local:     true,
		Streaming: true,
		Voices: []contracts.VoiceInfo{{
			ID:       "com.apple.voice.compact.en-US.Samantha",
			Name:     "Samantha",
			Locale:   "en-US",
			Provider: providerName,
		}},
		OutputFormats: []contracts.AudioFormat{{
			Encoding:     contracts.AudioEncodingPCM16,
			SampleRateHz: 24000,
			Channels:     1,
		}},
	}, nil
}

func (p *Provider) Synthesize(ctx context.Context, req contracts.SynthesisRequest) (contracts.TTSStream, error) {
	if runtime.GOOS != "darwin" && p.helperPath == "" {
		return nil, fmt.Errorf("%w: apple avspeech requires macOS", helper.ErrUnavailable)
	}
	path, err := p.resolveHelper()
	if err != nil {
		return nil, err
	}
	proc, err := (helper.Client{Path: path}).Start(ctx)
	if err != nil {
		return nil, err
	}
	if err := proc.Send(helper.TTSRequest(req)); err != nil {
		_ = proc.Close(ctx)
		return nil, err
	}
	stream := &ttsStream{proc: proc, events: make(chan contracts.TTSEvent, 16)}
	go stream.drain()
	return stream, nil
}

func (p *Provider) resolveHelper() (string, error) {
	if p.helperPath != "" {
		return p.helperPath, nil
	}
	return helper.ResolveBinary(envHelper, binaryName)
}

type ttsStream struct {
	proc   *helper.Process
	events chan contracts.TTSEvent
}

func (s *ttsStream) Events() <-chan contracts.TTSEvent { return s.events }

func (s *ttsStream) Close(ctx context.Context) error { return s.proc.Close(ctx) }

func (s *ttsStream) drain() {
	defer close(s.events)
	for raw := range s.proc.Events() {
		event, err := helper.TTSEvent(raw)
		if err != nil {
			s.events <- contracts.TTSEvent{Kind: contracts.TTSEventError, Error: err.Error()}
			continue
		}
		s.events <- event
		if event.Kind == contracts.TTSEventDone || event.Kind == contracts.TTSEventError {
			return
		}
	}
}
