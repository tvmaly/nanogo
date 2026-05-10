// Package realtime defines the normalized event contract used by voice providers.
package realtime

import (
	"encoding/json"
	"time"
)

const (
	EventSessionUpdate             = "session.update"
	EventConversationItemCreate    = "conversation.item.create"
	EventInputAudioBufferAppend    = "input_audio_buffer.append"
	EventInputAudioBufferCommit    = "input_audio_buffer.commit"
	EventInputAudioBufferClear     = "input_audio_buffer.clear"
	EventResponseCreate            = "response.create"
	EventResponseAudioDelta        = "response.audio.delta"
	EventResponseAudioDone         = "response.audio.done"
	EventResponseTextDelta         = "response.text.delta"
	EventResponseTextDone          = "response.text.done"
	EventResponseFunctionArgsDelta = "response.function_call_arguments.delta"
	EventResponseFunctionArgsDone  = "response.function_call_arguments.done"
	EventResponseDone              = "response.done"
	EventError                     = "error"
)

// Event is a provider-neutral, OpenAI Realtime-style event.
type Event struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"session_id,omitempty"`
	ItemID      string          `json:"item_id,omitempty"`
	ResponseID  string          `json:"response_id,omitempty"`
	AudioBase64 string          `json:"audio,omitempty"`
	Text        string          `json:"text,omitempty"`
	Name        string          `json:"name,omitempty"`
	ArgsJSON    json.RawMessage `json:"args_json,omitempty"`
	Session     *SessionConfig  `json:"session,omitempty"`
	Item        *Item           `json:"item,omitempty"`
	Error       string          `json:"error,omitempty"`
	Provider    string          `json:"provider,omitempty"`
	At          time.Time       `json:"at,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

// SessionConfig describes provider session settings.
type SessionConfig struct {
	Voice         string        `json:"voice,omitempty"`
	Instructions  string        `json:"instructions,omitempty"`
	TurnDetection TurnDetection `json:"turn_detection,omitempty"`
	AudioInput    AudioFormat   `json:"audio_input,omitempty"`
	AudioOutput   AudioFormat   `json:"audio_output,omitempty"`
}

type TurnDetection struct {
	Type string `json:"type,omitempty"`
}

type AudioFormat struct {
	Type string `json:"type,omitempty"`
	Rate int    `json:"rate,omitempty"`
}

// Item represents a conversation item.
type Item struct {
	Type    string    `json:"type"`
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Audio string `json:"audio,omitempty"`
}

// TextMessage creates a user text event.
func TextMessage(text string) Event {
	return Event{
		Type: EventConversationItemCreate,
		Item: &Item{
			Type: "message",
			Role: "user",
			Content: []Content{{
				Type: "input_text",
				Text: text,
			}},
		},
	}
}
