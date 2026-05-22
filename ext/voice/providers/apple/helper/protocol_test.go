package helper

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
)

func TestEncodeDecodeJSONLProtocol(t *testing.T) {
	line, err := EncodeRequest(Request{Type: "synthesize", SessionID: "s1", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), `"type":"synthesize"`) {
		t.Fatalf("encoded request = %s", line)
	}
	event, err := DecodeEvent([]byte(`{"type":"audio","session_id":"s1","audio_b64":"AQI="}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "audio" || event.SessionID != "s1" || event.AudioBase64 != "AQI=" {
		t.Fatalf("event = %#v", event)
	}
}

func TestTTSEventMapping(t *testing.T) {
	event, err := TTSEvent(Event{
		Type:        "audio",
		SessionID:   "s1",
		AudioBase64: base64.StdEncoding.EncodeToString([]byte{1, 2}),
		Sequence:    7,
		Format:      AudioFormat{Encoding: contracts.AudioEncodingPCM16, SampleRateHz: 24000, Channels: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != contracts.TTSEventAudio || string(event.PCM) != string([]byte{1, 2}) || event.Sequence != 7 {
		t.Fatalf("tts event = %#v", event)
	}
	if event.Format.Encoding != contracts.AudioEncodingPCM16 || event.Format.SampleRateHz != 24000 {
		t.Fatalf("format = %#v", event.Format)
	}
}

func TestTranscriptEventMapping(t *testing.T) {
	event, err := TranscriptEvent(Event{
		Type:       "final",
		SessionID:  "s1",
		Text:       "done",
		Locale:     "en-US",
		Confidence: 0.9,
		Sequence:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != contracts.TranscriptFinal || event.Text != "done" || !event.IsEndpoint {
		t.Fatalf("transcript event = %#v", event)
	}
}
