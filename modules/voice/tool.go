package voice

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/core/tools"
)

type StateProvider func() VoiceState

type ToolSource struct {
	tts   contracts.TextToSpeech
	sink  AudioSink
	state StateProvider
}

func NewToolSource(tts contracts.TextToSpeech, sink AudioSink, state StateProvider) *ToolSource {
	return &ToolSource{tts: tts, sink: sink, state: state}
}

func (s *ToolSource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	if s == nil || s.tts == nil || s.sink == nil {
		return nil, nil
	}
	if s.state != nil && !s.state().TTSEnabled {
		return nil, nil
	}
	return []tools.Tool{voiceSayTool{tts: s.tts, sink: s.sink}}, nil
}

type voiceSayTool struct {
	tts  contracts.TextToSpeech
	sink AudioSink
}

func (voiceSayTool) Name() string { return "voice_say" }

func (voiceSayTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"voice_say","description":"Speak text through the active voice sink.","parameters":{"type":"object","properties":{"text":{"type":"string"},"voice_id":{"type":"string"},"interrupt":{"type":"boolean"},"metadata":{"type":"object","additionalProperties":{"type":"string"}}},"required":["text"]}}}`)
}

func (t voiceSayTool) Call(ctx context.Context, raw json.RawMessage) (string, error) {
	var args struct {
		Text      string            `json:"text"`
		VoiceID   string            `json:"voice_id"`
		Metadata  map[string]string `json:"metadata"`
		Interrupt bool              `json:"interrupt"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("voice_say: decode args: %w", err)
	}
	if args.Text == "" {
		return "", fmt.Errorf("voice_say: text is required")
	}
	if t.tts == nil || t.sink == nil {
		return "voice unavailable; text only: " + args.Text, nil
	}
	stream, err := t.tts.Synthesize(ctx, contracts.SynthesisRequest{
		Text: args.Text,
		Options: contracts.TTSOptions{
			VoiceID:  args.VoiceID,
			Metadata: args.Metadata,
		},
	})
	if err != nil {
		return "", fmt.Errorf("voice_say: synthesize: %w", err)
	}
	defer stream.Close(ctx)
	for event := range stream.Events() {
		if err := t.sink.WriteTTS(ctx, event); err != nil {
			return "", fmt.Errorf("voice_say: write tts: %w", err)
		}
		if event.Kind == contracts.TTSEventDone {
			return "spoken: " + args.Text, nil
		}
		if event.Kind == contracts.TTSEventError {
			return "", fmt.Errorf("voice_say: tts error: %s", event.Error)
		}
	}
	return "spoken: " + args.Text, nil
}
