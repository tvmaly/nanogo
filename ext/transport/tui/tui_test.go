package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/skills"
)

type source struct{}

func (source) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return []tools.Tool{testTool{}}, nil
}

type testTool struct{}

func (testTool) Name() string { return "sample" }
func (testTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"sample"}}`)
}
func (testTool) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

type testRunner struct {
	got string
}

func (r *testRunner) RunSkill(_ context.Context, opts skills.RunSkillOpts) (string, error) {
	r.got = opts.SkillName
	return "ran", nil
}

type fakeCatalog struct {
	calls  int
	models []gateway.ModelInfo
}

func (f *fakeCatalog) ListModels(context.Context) ([]gateway.ModelInfo, error) {
	f.calls++
	return append([]gateway.ModelInfo(nil), f.models...), nil
}

type fakeVoice struct {
	state gateway.VoiceState
	err   error
}

func (f *fakeVoice) State(context.Context, string) (gateway.VoiceState, error) {
	return f.state, f.err
}

func (f *fakeVoice) Update(_ context.Context, _ string, patch gateway.VoicePatch) (gateway.VoiceState, error) {
	if f.err != nil {
		return f.state, f.err
	}
	if patch.STTEnabled != nil {
		f.state.STTEnabled = *patch.STTEnabled
	}
	if patch.TTSEnabled != nil {
		f.state.TTSEnabled = *patch.TTSEnabled
	}
	return f.state, nil
}

type fakeRealtime struct {
	state gateway.RealtimeVoiceState
	err   error
}

func (f *fakeRealtime) Start(context.Context) (gateway.RealtimeVoiceState, error) {
	if f.err != nil {
		return f.state, f.err
	}
	f.state.Connected = true
	return f.state, nil
}

func (f *fakeRealtime) Stop(context.Context) (gateway.RealtimeVoiceState, error) {
	if f.err != nil {
		return f.state, f.err
	}
	f.state.Connected = false
	return f.state, nil
}

func (f *fakeRealtime) Status(context.Context) (gateway.RealtimeVoiceState, error) {
	return f.state, f.err
}

func TestModelLoadsAndViewsPanes(t *testing.T) {
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{Provider: provider, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}, Model: "m"})
	m := NewModel(svc)
	msg := m.loadCmd()().(loadedMsg)
	next, _ := m.Update(msg)
	m = next.(Model)
	for range tabs {
		view := m.View()
		if !strings.Contains(view, tabs[m.active]) {
			t.Fatalf("view missing active tab: %s", view)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
	}
}

func TestModelChatUpdate(t *testing.T) {
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{Provider: provider, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
	m := NewModel(svc)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter did not produce chat command")
	}
	msg := cmd().(chatMsg)
	next, _ = m.Update(msg)
	m = next.(Model)
	if !strings.Contains(m.View(), "assistant: ok") {
		t.Fatalf("view = %s", m.View())
	}
}

func TestModelRendersGatewayErrors(t *testing.T) {
	svc := gateway.New(gateway.Config{Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus()})
	m := NewModel(svc)
	msg := m.loadCmd()().(loadedMsg)
	if msg.err == nil {
		t.Fatal("expected load error from missing tool source")
	}
	next, _ := m.Update(msg)
	m = next.(Model)
	if !strings.Contains(m.View(), "error:") {
		t.Fatalf("view = %s", m.View())
	}
	next, _ = m.Update(chatMsg{err: context.Canceled})
	m = next.(Model)
	if !strings.Contains(m.View(), "context canceled") {
		t.Fatalf("view = %s", m.View())
	}
}

func TestModelRendersStableEmptyStates(t *testing.T) {
	svc := gateway.New(gateway.Config{Provider: fakellm.New(nil), Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}, Model: "m"})
	m := NewModel(svc)
	msg := m.loadCmd()().(loadedMsg)
	next, _ := m.Update(msg)
	m = next.(Model)
	want := []string{"No chat yet.", "No sessions.", "No skills.", "sample", "turns=0", "No events."}
	for i, text := range want {
		m.active = i
		if view := m.View(); !strings.Contains(view, text) {
			t.Fatalf("pane %s view = %s, want %q", tabs[i], view, text)
		}
	}
}

func TestModelCostSlashUsesCurrentSession(t *testing.T) {
	costPath := filepath.Join(t.TempDir(), "cost.jsonl")
	body := strings.Join([]string{
		`{"session":"tui","input_tokens":10,"output_tokens":5,"cached_input_tokens":2,"cost_usd":0.25}`,
		`{"session":"other","input_tokens":99,"output_tokens":99,"cached_input_tokens":0,"cost_usd":9.99}`,
	}, "\n") + "\n"
	if err := os.WriteFile(costPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := gateway.New(gateway.Config{CostPath: costPath, Source: source{}, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus()})
	m := NewModel(svc)
	out, err := m.runSlash(context.Background(), "/cost")
	if err != nil {
		t.Fatalf("/cost: %v", err)
	}
	if !strings.Contains(out, "turns=1") || !strings.Contains(out, "input=10") || strings.Contains(out, "99") {
		t.Fatalf("cost output = %s", out)
	}
}

func TestModelSlashModelCommandsUseCacheAndSessionOverride(t *testing.T) {
	now := time.Unix(1000, 0)
	cat := &fakeCatalog{models: []gateway.ModelInfo{{ID: "m1"}, {ID: "m2"}}}
	svc := gateway.New(gateway.Config{Model: "default", ModelCatalog: cat, Source: source{}, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Now: func() time.Time { return now }})
	m := NewModel(svc)
	out, err := m.runSlash(context.Background(), "/model current")
	if err != nil || !strings.Contains(out, "default") {
		t.Fatalf("current out=%q err=%v", out, err)
	}
	out, err = m.runSlash(context.Background(), "/model list")
	if err != nil || !strings.Contains(out, "m1") || !strings.Contains(out, "m2") {
		t.Fatalf("list out=%q err=%v", out, err)
	}
	if _, err := m.runSlash(context.Background(), "/model list"); err != nil {
		t.Fatalf("cached list: %v", err)
	}
	if cat.calls != 1 {
		t.Fatalf("catalog calls = %d, want 1", cat.calls)
	}
	out, err = m.runSlash(context.Background(), "/model use m2")
	if err != nil || !strings.Contains(out, "m2") {
		t.Fatalf("use out=%q err=%v", out, err)
	}
	if got := svc.CurrentModel("tui"); got != "m2" {
		t.Fatalf("model = %q", got)
	}
	if got := svc.CurrentModel("other"); got != "default" {
		t.Fatalf("other model = %q", got)
	}
	if _, err := m.runSlash(context.Background(), "/model use missing"); err == nil {
		t.Fatal("expected invalid model error")
	}
	if out, err := m.runSlash(context.Background(), "/model flush"); err != nil || !strings.Contains(out, "flushed") {
		t.Fatalf("flush out=%q err=%v", out, err)
	}
	if _, err := m.runSlash(context.Background(), "/model list"); err != nil {
		t.Fatalf("list after flush: %v", err)
	}
	if cat.calls != 2 {
		t.Fatalf("catalog calls after flush = %d, want 2", cat.calls)
	}
}

func TestModelVoiceAndXAICommands(t *testing.T) {
	vc := &fakeVoice{}
	rt := &fakeRealtime{state: gateway.RealtimeVoiceState{Provider: "xai", Model: "grok", SessionID: "voice-1"}}
	svc := gateway.New(gateway.Config{Source: source{}, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Voice: vc, RealtimeVoice: rt})
	m := NewModel(svc)
	out, err := m.runSlash(context.Background(), "/stt on")
	if err != nil || !strings.Contains(out, "stt=true") {
		t.Fatalf("stt on out=%q err=%v", out, err)
	}
	out, err = m.runSlash(context.Background(), "/tts on")
	if err != nil || !strings.Contains(out, "tts=true") {
		t.Fatalf("tts on out=%q err=%v", out, err)
	}
	out, err = m.runSlash(context.Background(), "/xai on")
	if err != nil || !strings.Contains(out, "provider=xai") || !strings.Contains(out, "connected=true") {
		t.Fatalf("xai on out=%q err=%v", out, err)
	}
	out, err = m.runSlash(context.Background(), "/xai status")
	if err != nil || !strings.Contains(out, "session=voice-1") {
		t.Fatalf("xai status out=%q err=%v", out, err)
	}
	out, err = m.runSlash(context.Background(), "/xai off")
	if err != nil || !strings.Contains(out, "connected=false") {
		t.Fatalf("xai off out=%q err=%v", out, err)
	}
}

func TestModelSessionsSkillsStreamingAndBoundedEvents(t *testing.T) {
	runner := &testRunner{}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte("---\nname: demo\ndescription: Demo\n---\nRun it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := gateway.New(gateway.Config{Provider: fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}}), Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}, SkillsDir: dir, SkillRunner: runner})
	_, _ = svc.CreateSession("tui")
	_, _ = svc.CreateSession("other")
	m := NewModelWithConfig(svc, Config{EventLimit: 2})
	msg := m.loadCmd()().(loadedMsg)
	next, _ := m.Update(msg)
	m = next.(Model)
	m.active = 1
	for i, sess := range m.sessions {
		if sess.ID == "other" {
			m.selectedSession = i
		}
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.session != "other" {
		t.Fatalf("selected session = %q", m.session)
	}
	m.active = 2
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("skill enter produced nil cmd")
	}
	msg2 := cmd().(commandMsg)
	next, _ = m.Update(msg2)
	m = next.(Model)
	if runner.got != "demo" || !strings.Contains(strings.Join(m.chat, "\n"), "skill ran: demo") {
		t.Fatalf("runner=%q chat=%#v view=%s", runner.got, m.chat, m.View())
	}
	m.active = 0
	next, _ = m.Update(streamDeltaMsg{delta: "he"})
	m = next.(Model)
	next, _ = m.Update(streamDeltaMsg{delta: "llo"})
	m = next.(Model)
	if !m.streaming || !strings.Contains(m.View(), "hello") {
		t.Fatalf("stream view = %s", m.View())
	}
	next, _ = m.Update(streamDoneMsg{})
	m = next.(Model)
	if m.streaming || !strings.Contains(m.View(), "assistant: hello") {
		t.Fatalf("done view = %s", m.View())
	}
	m.appendEvents(gateway.EventRecord{Seq: 1, Event: "a"}, gateway.EventRecord{Seq: 2, Event: "b"}, gateway.EventRecord{Seq: 3, Event: "c"})
	if len(m.events) != 2 || m.events[0].Seq != 2 {
		t.Fatalf("events = %#v", m.events)
	}
}
