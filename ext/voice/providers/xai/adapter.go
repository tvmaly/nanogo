package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

const (
	DefaultModel = "grok-voice-think-fast-1.0"
	DefaultVoice = "eve"
)

type Config struct {
	APIKey       string
	Model        string
	URL          string
	Voice        string
	Instructions string
}

type Adapter struct {
	cfg  Config
	dial func(context.Context, string, map[string]string) (realtime.RealtimeConn, error)
}

func New(cfg Config) *Adapter {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Voice == "" {
		cfg.Voice = DefaultVoice
	}
	return &Adapter{
		cfg: cfg,
		dial: func(ctx context.Context, url string, headers map[string]string) (realtime.RealtimeConn, error) {
			return realtime.DialWebSocket(ctx, url, headers)
		},
	}
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		APIKey: os.Getenv("XAI_API_KEY"),
		Model:  os.Getenv("XAI_REALTIME_MODEL"),
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("XAI_API_KEY is required")
	}
	cfg.URL = URLForModel(cfg.Model)
	cfg.Voice = DefaultVoice
	return cfg, nil
}

func URLForModel(model string) string {
	if model == "" {
		model = DefaultModel
	}
	return "wss://api.x.ai/v1/realtime?model=" + model
}

func (a *Adapter) Name() string { return "xai" }

func (a *Adapter) Connect(ctx context.Context, cfg realtime.ProviderConfig) (realtime.RealtimeConn, error) {
	apiKey := first(cfg.APIKey, a.cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("xai: XAI_API_KEY is required")
	}
	model := first(cfg.Model, a.cfg.Model, DefaultModel)
	url := first(cfg.URL, a.cfg.URL, URLForModel(model))
	headers := map[string]string{"Authorization": "Bearer " + apiKey}
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	conn, err := a.dial(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	return mappedConn{RealtimeConn: conn}, nil
}

func SessionUpdate(cfg Config) realtime.Event {
	voice := first(cfg.Voice, DefaultVoice)
	instructions := first(cfg.Instructions, "You are a helpful voice tutor.")
	return realtime.Event{
		Type: realtime.EventSessionUpdate,
		Session: &realtime.SessionConfig{
			Voice:         voice,
			Instructions:  instructions,
			TurnDetection: realtime.TurnDetection{Type: "server_vad"},
			AudioInput:    realtime.AudioFormat{Type: "audio/pcm", Rate: 24000},
			AudioOutput:   realtime.AudioFormat{Type: "audio/pcm", Rate: 24000},
		},
	}
}

func MapWire(raw []byte) realtime.Event {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		return realtime.Event{Type: realtime.EventError, Error: "xai: decode event: " + err.Error(), Provider: "xai", Raw: append([]byte(nil), raw...)}
	}
	var typ string
	_ = json.Unmarshal(wire["type"], &typ)
	event := realtime.Event{Type: typ, Provider: "xai", Raw: append([]byte(nil), raw...)}
	_ = json.Unmarshal(wire["text"], &event.Text)
	_ = json.Unmarshal(wire["audio"], &event.AudioBase64)
	var errObj struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if errRaw := wire["error"]; len(errRaw) > 0 {
		if err := json.Unmarshal(errRaw, &errObj); err == nil {
			if errObj.Message != "" {
				event.Error = errObj.Message
			} else if errObj.Code != "" {
				event.Error = errObj.Code
			}
		}
	}
	switch typ {
	case "response.output_audio.delta":
		event.Type = realtime.EventResponseAudioDelta
		_ = json.Unmarshal(wire["delta"], &event.AudioBase64)
	case "response.output_audio.done":
		event.Type = realtime.EventResponseAudioDone
	case "response.output_text.delta":
		event.Type = realtime.EventResponseTextDelta
		_ = json.Unmarshal(wire["delta"], &event.Text)
	case "response.output_text.done":
		event.Type = realtime.EventResponseTextDone
	case "response.done":
		event.Type = realtime.EventResponseDone
	case "error":
		event.Type = realtime.EventError
		if event.Error == "" {
			event.Error = "xai provider error"
		}
	}
	return event
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type mappedConn struct {
	realtime.RealtimeConn
}

func (c mappedConn) Receive(ctx context.Context) (realtime.Event, error) {
	event, err := c.RealtimeConn.Receive(ctx)
	if err != nil {
		return event, err
	}
	if len(event.Raw) > 0 {
		return MapWire(event.Raw), nil
	}
	return event, nil
}
