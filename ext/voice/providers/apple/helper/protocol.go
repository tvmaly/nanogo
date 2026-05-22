package helper

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/contracts"
)

type AudioFormat struct {
	Encoding     contracts.AudioEncoding `json:"encoding,omitempty"`
	SampleRateHz int                     `json:"sample_rate_hz,omitempty"`
	Channels     int                     `json:"channels,omitempty"`
}

type Request struct {
	Type           string            `json:"type"`
	SessionID      string            `json:"session_id,omitempty"`
	Text           string            `json:"text,omitempty"`
	Locale         string            `json:"locale,omitempty"`
	VoiceID        string            `json:"voice_id,omitempty"`
	Rate           float64           `json:"rate,omitempty"`
	Pitch          float64           `json:"pitch,omitempty"`
	Volume         float64           `json:"volume,omitempty"`
	InterimResults bool              `json:"interim_results,omitempty"`
	AudioBase64    string            `json:"audio_b64,omitempty"`
	Final          bool              `json:"final,omitempty"`
	Format         AudioFormat       `json:"format,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type Event struct {
	Type        string                `json:"type"`
	SessionID   string                `json:"session_id,omitempty"`
	Text        string                `json:"text,omitempty"`
	Locale      string                `json:"locale,omitempty"`
	Confidence  float64               `json:"confidence,omitempty"`
	AudioBase64 string                `json:"audio_b64,omitempty"`
	Error       string                `json:"error,omitempty"`
	Sequence    int64                 `json:"sequence,omitempty"`
	Format      AudioFormat           `json:"format,omitempty"`
	Voices      []contracts.VoiceInfo `json:"voices,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
}

func EncodeRequest(req Request) ([]byte, error) {
	if req.Type == "" {
		return nil, fmt.Errorf("apple voice helper request type is required")
	}
	return json.Marshal(req)
}

func DecodeEvent(line []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(line, &event); err != nil {
		return Event{}, fmt.Errorf("decode apple voice helper event: %w", err)
	}
	if event.Type == "" {
		return Event{}, fmt.Errorf("apple voice helper event type is required")
	}
	return event, nil
}

func FormatFromContracts(format contracts.AudioFormat) AudioFormat {
	return AudioFormat{Encoding: format.Encoding, SampleRateHz: format.SampleRateHz, Channels: format.Channels}
}

func FormatToContracts(format AudioFormat) contracts.AudioFormat {
	return contracts.AudioFormat{Encoding: format.Encoding, SampleRateHz: format.SampleRateHz, Channels: format.Channels}
}

func TTSRequest(req contracts.SynthesisRequest) Request {
	return Request{
		Type:      "synthesize",
		SessionID: req.SessionID,
		Text:      req.Text,
		Locale:    req.Options.Locale,
		VoiceID:   req.Options.VoiceID,
		Rate:      req.Options.Rate,
		Pitch:     req.Options.Pitch,
		Volume:    req.Options.Volume,
		Format:    FormatFromContracts(req.Options.OutputFormat),
		Metadata:  req.Options.Metadata,
	}
}

func TTSEvent(event Event) (contracts.TTSEvent, error) {
	out := contracts.TTSEvent{
		SessionID: event.SessionID,
		Format:    FormatToContracts(event.Format),
		Error:     event.Error,
		Sequence:  event.Sequence,
		Metadata:  event.Metadata,
	}
	switch event.Type {
	case "started":
		out.Kind = contracts.TTSEventStarted
	case "audio":
		out.Kind = contracts.TTSEventAudio
		if event.AudioBase64 != "" {
			pcm, err := base64.StdEncoding.DecodeString(event.AudioBase64)
			if err != nil {
				return contracts.TTSEvent{}, fmt.Errorf("decode apple tts audio: %w", err)
			}
			out.PCM = pcm
		}
	case "marker":
		out.Kind = contracts.TTSEventMarker
		out.Marker = event.Text
	case "done":
		out.Kind = contracts.TTSEventDone
	case "error":
		out.Kind = contracts.TTSEventError
	default:
		return contracts.TTSEvent{}, fmt.Errorf("unknown apple tts helper event type %q", event.Type)
	}
	return out, nil
}

func STTStartRequest(opts contracts.STTOptions) Request {
	return Request{
		Type:           "start",
		SessionID:      opts.SessionID,
		Locale:         opts.Locale,
		InterimResults: opts.InterimResults,
		Format:         FormatFromContracts(opts.InputFormat),
		Metadata:       opts.Metadata,
	}
}

func STTAudioRequest(frame contracts.AudioFrame) Request {
	return Request{
		Type:        "audio",
		SessionID:   frame.SessionID,
		AudioBase64: base64.StdEncoding.EncodeToString(frame.PCM),
		Final:       frame.Final,
		Format:      FormatFromContracts(frame.Format),
	}
}

func TranscriptEvent(event Event) (contracts.TranscriptEvent, error) {
	out := contracts.TranscriptEvent{
		SessionID:  event.SessionID,
		Text:       event.Text,
		Locale:     event.Locale,
		Confidence: event.Confidence,
		Sequence:   event.Sequence,
		Metadata:   event.Metadata,
	}
	switch event.Type {
	case "partial":
		out.Kind = contracts.TranscriptPartial
	case "final":
		out.Kind = contracts.TranscriptFinal
		out.IsEndpoint = true
	case "error":
		out.Kind = contracts.TranscriptFinal
		out.Metadata = cloneMetadata(event.Metadata)
		if out.Metadata == nil {
			out.Metadata = map[string]string{}
		}
		out.Metadata["error"] = event.Error
	default:
		return contracts.TranscriptEvent{}, fmt.Errorf("unknown apple stt helper event type %q", event.Type)
	}
	return out, nil
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
