package contracts

import (
	"context"
	"time"
)

type AudioEncoding string

const (
	AudioEncodingPCM16   AudioEncoding = "pcm16"
	AudioEncodingFloat32 AudioEncoding = "float32"
	AudioEncodingWAV     AudioEncoding = "wav"
)

type AudioFormat struct {
	Encoding      AudioEncoding
	SampleRateHz  int
	Channels      int
	FrameDuration time.Duration
}

type AudioFrame struct {
	SessionID string
	Format    AudioFormat
	PCM       []byte
	Final     bool
	Timestamp time.Duration
}

type TranscriptKind string

const (
	TranscriptPartial TranscriptKind = "partial"
	TranscriptFinal   TranscriptKind = "final"
)

type TranscriptEvent struct {
	SessionID  string
	Kind       TranscriptKind
	Text       string
	Locale     string
	Confidence float64
	Start      time.Duration
	End        time.Duration
	Sequence   int64
	IsEndpoint bool
	Metadata   map[string]string
}

type STTOptions struct {
	SessionID       string
	Locale          string
	InputFormat     AudioFormat
	InterimResults  bool
	WordTimestamps  bool
	EndpointingMode string
	Metadata        map[string]string
}

type STTCapabilities struct {
	Provider        string
	Local           bool
	Streaming       bool
	OfflineFiles    bool
	Locales         []string
	InputFormats    []AudioFormat
	SupportsPartial bool
	SupportsTiming  bool
}

type SpeechToText interface {
	Capabilities(ctx context.Context) (STTCapabilities, error)
	OpenSTTSession(ctx context.Context, opts STTOptions) (STTSession, error)
}

type STTSession interface {
	WriteAudio(ctx context.Context, frame AudioFrame) error
	Events() <-chan TranscriptEvent
	Close(ctx context.Context) error
}

type TTSOptions struct {
	VoiceID      string
	Locale       string
	Rate         float64
	Pitch        float64
	Volume       float64
	OutputFormat AudioFormat
	Metadata     map[string]string
}

type SynthesisRequest struct {
	SessionID string
	Text      string
	Options   TTSOptions
}

type TTSEventKind string

const (
	TTSEventStarted TTSEventKind = "started"
	TTSEventAudio   TTSEventKind = "audio"
	TTSEventMarker  TTSEventKind = "marker"
	TTSEventDone    TTSEventKind = "done"
	TTSEventError   TTSEventKind = "error"
)

type TTSEvent struct {
	SessionID string
	Kind      TTSEventKind
	Format    AudioFormat
	PCM       []byte
	TextRange [2]int
	Marker    string
	Error     string
	Sequence  int64
	Metadata  map[string]string
}

type VoiceInfo struct {
	ID       string
	Name     string
	Locale   string
	Gender   string
	Provider string
	Metadata map[string]string
}

type TTSCapabilities struct {
	Provider      string
	Local         bool
	Streaming     bool
	Voices        []VoiceInfo
	OutputFormats []AudioFormat
	SupportsSSML  bool
}

type TextToSpeech interface {
	Capabilities(ctx context.Context) (TTSCapabilities, error)
	Synthesize(ctx context.Context, req SynthesisRequest) (TTSStream, error)
}

type TTSStream interface {
	Events() <-chan TTSEvent
	Close(ctx context.Context) error
}
