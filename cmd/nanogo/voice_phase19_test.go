package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/ext/voice/localaudio"
	voice "github.com/tvmaly/nanogo/modules/voice"
)

func TestVoiceProvidersCommandListsConfiguredProviders(t *testing.T) {
	var out bytes.Buffer
	err := runVoiceProviders(context.Background(), &out, voiceProviderRegistry{
		STT: map[string]contracts.SpeechToText{"fake": contractfake.NewSpeechToText()},
		TTS: map[string]contracts.TextToSpeech{"fake": contractfake.NewTextToSpeech()},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "stt fake") || !strings.Contains(got, "tts fake") {
		t.Fatalf("output = %q", got)
	}
}

func TestVoiceSTTFileCommandPrintsFinalTranscript(t *testing.T) {
	var out bytes.Buffer
	provider := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"})
	if err := runVoiceSTT(context.Background(), &out, provider, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestVoiceTTSCommandWritesToSink(t *testing.T) {
	sink := &recordingSink{}
	if err := runVoiceTTS(context.Background(), contractfake.NewTextToSpeech(), sink, "hello"); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) == 0 {
		t.Fatal("expected tts events")
	}
}

type recordingSink struct {
	events []contracts.TTSEvent
}

func (s *recordingSink) WriteTTS(_ context.Context, event contracts.TTSEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingSink) Close(context.Context) error { return nil }

func TestVoiceChatRoutesFinalTranscriptToAgentAndTTS(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	stt := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "what is a magnet"})
	tts := contractfake.NewTextToSpeech()
	agent := &fakeVoiceAgent{reply: "A magnet pulls some metals."}

	if err := runVoiceChat(context.Background(), capture, playback, stt, tts, agent, "en-US", voiceChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(agent.requests) != 1 || agent.requests[0].Text != "what is a magnet" {
		t.Fatalf("agent requests = %#v", agent.requests)
	}
	if string(playback.Writes()[0]) != "A magnet pulls some metals." {
		t.Fatalf("playback writes = %#v", playback.Writes())
	}
}

func TestVoiceChatDebugReportsPipelineStages(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	stt := contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "hello"})
	tts := contractfake.NewTextToSpeech()
	agent := &fakeVoiceAgent{reply: "hi"}
	var out bytes.Buffer

	if err := runVoiceChat(context.Background(), capture, playback, stt, tts, agent, "en-US", voiceChatOptions{DebugOut: &out}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"capture bytes=", `tts audio bytes=2`, "tts done"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug output = %q, missing %q", got, want)
		}
	}
}

func TestDebouncedSpeechToTextEmitsLatestFinalAfterIdle(t *testing.T) {
	inner := contractfake.NewSpeechToText(
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "Can"},
		contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "Can you"},
	)
	wrapped := debouncedSpeechToText{inner: inner, delay: 10 * time.Millisecond}
	session, err := wrapped.OpenSTTSession(context.Background(), contracts.STTOptions{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{PCM: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := session.WriteAudio(context.Background(), contracts.AudioFrame{PCM: []byte{2}}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-session.Events():
		if event.Text != "Can you" {
			t.Fatalf("event text = %q", event.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced transcript")
	}
}

func TestAgentVoiceGatewayRunsFullAgentLoopAndSpeaksAnswer(t *testing.T) {
	var debug bytes.Buffer
	gateway := newAgentVoiceGateway(agentVoiceGatewayConfig{
		Cfg:      testVoiceGatewayConfig(),
		Provider: fakellm.New([]llm.Chunk{{TextDelta: "The answer is four.", FinishReason: "stop"}}),
		Model:    "test-model",
		DebugOut: &debug,
	})
	result, err := gateway.SubmitVoiceText(context.Background(), voice.VoiceAgentRequest{SessionID: "voice-chat", Text: "What is 2+2?"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "The answer is four." {
		t.Fatalf("result text = %q", result.Text)
	}
	if !strings.Contains(debug.String(), `voice chat stt final="What is 2+2?"`) {
		t.Fatalf("debug = %q", debug.String())
	}
}

func TestAgentVoiceGatewayExecutesToolsAndSpeaksFinalAnswer(t *testing.T) {
	provider := fakellm.New(
		[]llm.Chunk{
			{ToolCall: &llm.ToolCall{ID: "tc1", Name: "read_file", Args: json.RawMessage(`{"path":"/tmp/phase-19-6.txt"}`)}},
			{FinishReason: "tool_calls"},
		},
		[]llm.Chunk{{TextDelta: "I read the file.", FinishReason: "stop"}},
	)
	gateway := newTestAgentVoiceGateway(t, provider)
	result, err := gateway.SubmitVoiceText(context.Background(), voice.VoiceAgentRequest{SessionID: "voice-chat", Text: "Use a tool"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "I read the file." {
		t.Fatalf("result text = %q", result.Text)
	}
	if provider.Calls != 2 {
		t.Fatalf("llm calls = %d, want 2", provider.Calls)
	}
	if !strings.Contains(gateway.debug.(*bytes.Buffer).String(), "voice chat tool call=read_file") {
		t.Fatalf("debug = %q", gateway.debug.(*bytes.Buffer).String())
	}
}

func TestVoiceToolSourceExcludesAskUser(t *testing.T) {
	gateway := newTestAgentVoiceGateway(t, fakellm.New([]llm.Chunk{{TextDelta: "ok", FinishReason: "stop"}}))
	list, err := gateway.toolSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list {
		if tool.Name() == "ask_user" {
			t.Fatalf("ask_user must not be exposed in voice chat")
		}
	}
}

func TestVoiceWebSearchDefaultsInjectedIntoOpenAIConfig(t *testing.T) {
	cfg := &config{}
	cfg.LLM.Driver = "openai"
	cfg.LLM.Config = json.RawMessage(`{"base_url":"https://openrouter.ai/api/v1","model":"m","server_tools":[{"type":"existing"}]}`)
	if err := enableVoiceWebSearch(cfg); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ServerTools []json.RawMessage `json:"server_tools"`
	}
	if err := json.Unmarshal(cfg.LLM.Config, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ServerTools) != 2 {
		t.Fatalf("server_tools len = %d, want 2", len(got.ServerTools))
	}
	if !strings.Contains(string(got.ServerTools[1]), `"openrouter:web_search"`) ||
		!strings.Contains(string(got.ServerTools[1]), `"max_results":8`) ||
		!strings.Contains(string(got.ServerTools[1]), `"max_total_results":20`) {
		t.Fatalf("web search tool = %s", got.ServerTools[1])
	}
}

func TestVoiceWebSearchAdvancedConfigPassthrough(t *testing.T) {
	cfg := &config{}
	cfg.LLM.Driver = "openai"
	cfg.LLM.Config = json.RawMessage(`{"base_url":"https://openrouter.ai/api/v1","model":"m"}`)
	cfg.Voice.WebSearch.Engine = "parallel"
	cfg.Voice.WebSearch.MaxResults = 3
	cfg.Voice.WebSearch.MaxTotalResults = 9
	cfg.Voice.WebSearch.SearchContextSize = "high"
	cfg.Voice.WebSearch.AllowedDomains = []string{"arxiv.org", "nature.com"}
	cfg.Voice.WebSearch.ExcludedDomains = []string{"reddit.com"}
	if err := enableVoiceWebSearch(cfg); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ServerTools []struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		} `json:"server_tools"`
	}
	if err := json.Unmarshal(cfg.LLM.Config, &got); err != nil {
		t.Fatal(err)
	}
	params := got.ServerTools[0].Parameters
	if got.ServerTools[0].Type != "openrouter:web_search" ||
		params["engine"] != "parallel" ||
		params["search_context_size"] != "high" ||
		int(params["max_results"].(float64)) != 3 ||
		int(params["max_total_results"].(float64)) != 9 {
		t.Fatalf("server tool = %#v", got.ServerTools[0])
	}
	if fmt.Sprint(params["allowed_domains"]) != "[arxiv.org nature.com]" ||
		fmt.Sprint(params["excluded_domains"]) != "[reddit.com]" {
		t.Fatalf("domains = %#v", params)
	}
}

func TestVoiceWebSearchInjectedIntoRouterVoiceRoute(t *testing.T) {
	cfg := &config{}
	cfg.LLM.Driver = "router"
	cfg.LLM.Config = json.RawMessage(`{
		"providers":{
			"voice":{"driver":"openai","config":{"model":"voice-model"}},
			"other":{"driver":"fake","config":{}}
		},
		"rules":[{"when":"source=voice","route":"voice"}],
		"fallback":"other"
	}`)
	if err := enableVoiceWebSearch(cfg); err != nil {
		t.Fatal(err)
	}
	var rc struct {
		Providers map[string]struct {
			Driver string          `json:"driver"`
			Config json.RawMessage `json:"config"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(cfg.LLM.Config, &rc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rc.Providers["voice"].Config), "openrouter:web_search") {
		t.Fatalf("voice provider config = %s", rc.Providers["voice"].Config)
	}
	if strings.Contains(string(rc.Providers["other"].Config), "openrouter:web_search") {
		t.Fatalf("non-openai provider was modified: %s", rc.Providers["other"].Config)
	}
}

func newTestAgentVoiceGateway(t *testing.T, provider llm.Provider) *agentVoiceGateway {
	t.Helper()
	var debug bytes.Buffer
	return newAgentVoiceGateway(agentVoiceGatewayConfig{
		Cfg:      testVoiceGatewayConfig(),
		Provider: provider,
		Model:    "test-model",
		DebugOut: &debug,
	})
}

func testVoiceGatewayConfig() *config {
	cfg := &config{}
	cfg.LLM.Driver = "openai"
	cfg.LLM.Config = json.RawMessage(`{"model":"test-model"}`)
	return cfg
}

type fakeVoiceAgent struct {
	reply    string
	requests []voice.VoiceAgentRequest
}

func (a *fakeVoiceAgent) SubmitVoiceText(_ context.Context, req voice.VoiceAgentRequest) (voice.VoiceAgentResult, error) {
	a.requests = append(a.requests, req)
	return voice.VoiceAgentResult{Text: a.reply}, nil
}
