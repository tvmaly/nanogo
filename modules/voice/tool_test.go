package voice

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	"github.com/tvmaly/nanogo/core/tools"
)

func TestVoiceSayToolAvailabilityFollowsRuntimeTTSState(t *testing.T) {
	state := VoiceState{TTSEnabled: false}
	source := NewToolSource(contractfake.NewTextToSpeech(), &fakeSink{}, func() VoiceState { return state })
	list, err := source.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("tools with TTS disabled = %#v", list)
	}

	state.TTSEnabled = true
	list, err = source.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name() != "voice_say" {
		t.Fatalf("tools with TTS enabled = %#v", list)
	}
}

func TestVoiceSayToolSynthesizesAndWritesToSink(t *testing.T) {
	sink := &fakeSink{}
	source := NewToolSource(contractfake.NewTextToSpeech(), sink, func() VoiceState {
		return VoiceState{TTSEnabled: true}
	})
	list, err := source.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"text": "Great job."})
	got, err := list[0].Call(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "spoken: Great job.") {
		t.Fatalf("result = %q", got)
	}
	if len(sink.events) == 0 || sink.events[0].Kind != contracts.TTSEventStarted {
		t.Fatalf("sink events = %#v", sink.events)
	}
}
