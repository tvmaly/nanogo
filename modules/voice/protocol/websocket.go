package protocol

import "github.com/tvmaly/nanogo/core/contracts"

const (
	EventSessionStart = "voice.session.start"
	EventStateUpdate  = "voice.state.update"
	EventStateUpdated = "voice.state.updated"
	EventAudioEnd     = "voice.audio.end"
	EventCancel       = "voice.cancel"
	EventTextInput    = "voice.text.input"
	EventSTTPartial   = "stt.partial"
	EventSTTFinal     = "stt.final"
	EventAgentFinal   = "agent.final"
	EventTTSStarted   = "tts.started"
	EventTTSDone      = "tts.done"
	EventError        = "voice.error"
)

type ControlEvent struct {
	Type                    string                 `json:"type"`
	SessionID               string                 `json:"session_id,omitempty"`
	Locale                  string                 `json:"locale,omitempty"`
	InputFormat             *contracts.AudioFormat `json:"input_format,omitempty"`
	STTProvider             string                 `json:"stt_provider,omitempty"`
	TTSProvider             string                 `json:"tts_provider,omitempty"`
	STTEnabled              *bool                  `json:"stt_enabled,omitempty"`
	TTSEnabled              *bool                  `json:"tts_enabled,omitempty"`
	AutoSpeakFinalResponses *bool                  `json:"auto_speak_final_responses,omitempty"`
	Text                    string                 `json:"text,omitempty"`
	Sequence                int64                  `json:"sequence,omitempty"`
	Message                 string                 `json:"message,omitempty"`
}
