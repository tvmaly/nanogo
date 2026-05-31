package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	modulesession "github.com/tvmaly/nanogo/modules/session"
	"github.com/tvmaly/nanogo/modules/skills"
)

type oneTool struct{}

func (oneTool) Name() string { return "sample" }
func (oneTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"sample"}}`)
}
func (oneTool) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

type source struct{}

func (source) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return []tools.Tool{oneTool{}}, nil
}

type runner struct{ got skills.RunSkillOpts }

func (r *runner) RunSkill(_ context.Context, opts skills.RunSkillOpts) (string, error) {
	r.got = opts
	return "ran", nil
}

func TestRegistryDispatchErrors(t *testing.T) {
	t.Parallel()
	svc := New(Config{})
	_, err := svc.Dispatch(context.Background(), Request{Method: "missing"})
	if AsError(err).Code != CodeUnknownMethod {
		t.Fatalf("missing code = %v, want %s", AsError(err).Code, CodeUnknownMethod)
	}
	_, err = svc.Dispatch(context.Background(), Request{Method: "voice.start"})
	if AsError(err).Code != CodeUnsupported {
		t.Fatalf("reserved code = %v, want %s", AsError(err).Code, CodeUnsupported)
	}
}

func TestRegistryDispatchesStatus(t *testing.T) {
	t.Parallel()
	svc := New(Config{Model: "test-model"})
	payload, err := svc.Dispatch(context.Background(), Request{Method: "status"})
	if err != nil {
		t.Fatalf("Dispatch(status): %v", err)
	}
	status, ok := payload.(Status)
	if !ok {
		t.Fatalf("payload type = %T, want gateway.Status", payload)
	}
	if !status.OK || status.Model != "test-model" {
		t.Fatalf("status = %#v", status)
	}
	if !contains(status.Methods, "agent") || !contains(status.Methods, "events.subscribe") {
		t.Fatalf("methods = %#v", status.Methods)
	}
}

func TestGatewayRegistersOperationsByFeatureFamily(t *testing.T) {
	t.Parallel()
	methods := New(Config{}).Status().Methods
	families := map[string][]string{
		"status/diagnostics": {"status"},
		"chat/sessions/events": {
			"agent",
			"sessions.create",
			"sessions.get",
			"sessions.messages",
			"sessions.list",
			"sessions.delete",
			"events.subscribe",
		},
		"skills/tools": {
			"skills.list",
			"skills.run",
			"tools.catalog",
			"tools.effective",
		},
		"costs/models": {
			"costs.summary",
			"models.current",
			"models.use",
			"models.list",
			"models.flush",
		},
		"voice/realtime": {
			"voice.state",
			"voice.update",
			"xai.start",
			"xai.stop",
			"xai.status",
		},
		"help": {
			"help.search",
			"help.topic",
			"help.suggest",
			"help.render",
			"help.validate",
		},
	}
	for family, want := range families {
		for _, method := range want {
			if !contains(methods, method) {
				t.Fatalf("%s methods = %v, missing %q", family, methods, method)
			}
		}
	}
}

func TestReservedNamespacesReturnUnsupported(t *testing.T) {
	t.Parallel()
	svc := New(Config{})
	for _, method := range []string{"voice.start", "memory.search", "approvals.list"} {
		_, err := svc.Dispatch(context.Background(), Request{Method: method})
		if AsError(err).Code != CodeUnsupported {
			t.Fatalf("%s code = %v, want %s", method, AsError(err).Code, CodeUnsupported)
		}
	}
}

func TestSubmitChatCollectsEventsAndMessages(t *testing.T) {
	t.Parallel()
	provider := fakellm.New([]llm.Chunk{
		{TextDelta: "hel"},
		{TextDelta: "lo"},
		{FinishReason: "stop", Usage: &llm.Usage{InputTokens: 3, OutputTokens: 2}},
	})
	svc := New(Config{
		Provider: provider,
		Store:    modulesession.NewStore(t.TempDir(), nil),
		Bus:      event.NewBus(),
		Source:   source{},
		Model:    "test-model",
	})
	got, err := svc.SubmitChat(context.Background(), ChatRequest{Session: "s1", Message: "hi"})
	if err != nil {
		t.Fatalf("SubmitChat: %v", err)
	}
	if got.Text != "hello" {
		t.Fatalf("text = %q, want hello", got.Text)
	}
	info, err := svc.GetSession("s1", true)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(info.Messages) != 2 || info.Messages[1].Content != "hello" {
		t.Fatalf("messages = %#v", info.Messages)
	}
}

func TestStreamChatEmitsDeltasThenDone(t *testing.T) {
	t.Parallel()
	provider := fakellm.New([]llm.Chunk{
		{TextDelta: "a"},
		{TextDelta: "b"},
		{FinishReason: "stop"},
	})
	svc := New(Config{
		Provider: provider,
		Store:    modulesession.NewStore(t.TempDir(), nil),
		Bus:      event.NewBus(),
		Source:   source{},
	})
	ch, err := svc.StreamChat(context.Background(), ChatRequest{Session: "stream", Message: "hi"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	var kinds []string
	var text strings.Builder
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		kinds = append(kinds, ev.Kind)
		text.WriteString(ev.Delta)
		if ev.Kind == "done" && ev.Text != "ab" {
			t.Fatalf("done text = %q, want ab", ev.Text)
		}
	}
	if got := strings.Join(kinds, ","); got != "delta,delta,done" {
		t.Fatalf("kinds = %s", got)
	}
	if text.String() != "ab" {
		t.Fatalf("delta text = %q", text.String())
	}
}

func TestSessionOperationsCreateInspectMessagesAndDelete(t *testing.T) {
	t.Parallel()
	svc := New(Config{Store: modulesession.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
	payload, err := svc.Dispatch(context.Background(), Request{Method: "sessions.create", Params: json.RawMessage(`{"id":"s1"}`)})
	if err != nil {
		t.Fatalf("sessions.create: %v", err)
	}
	info := payload.(SessionInfo)
	if info.ID != "s1" || info.Status != session.StatusActive {
		t.Fatalf("created = %#v", info)
	}
	if _, err := svc.Dispatch(context.Background(), Request{Method: "sessions.get", Params: json.RawMessage(`{"id":"s1"}`)}); err != nil {
		t.Fatalf("sessions.get: %v", err)
	}
	payload, err = svc.Dispatch(context.Background(), Request{Method: "sessions.list"})
	if err != nil {
		t.Fatalf("sessions.list: %v", err)
	}
	if got := payload.([]SessionInfo); len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("list = %#v", got)
	}
	payload, err = svc.Dispatch(context.Background(), Request{Method: "sessions.messages", Params: json.RawMessage(`{"id":"s1"}`)})
	if err != nil {
		t.Fatalf("sessions.messages: %v", err)
	}
	if got := payload.(SessionInfo); len(got.Messages) != 0 {
		t.Fatalf("messages = %#v", got.Messages)
	}
	payload, err = svc.Dispatch(context.Background(), Request{Method: "sessions.delete", Params: json.RawMessage(`{"id":"s1"}`)})
	if err != nil {
		t.Fatalf("sessions.delete: %v", err)
	}
	if deleted := payload.(map[string]bool)["deleted"]; !deleted {
		t.Fatalf("deleted payload = %#v", payload)
	}
}

func TestSkillsListAndRunUseExistingSkillsModule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte("---\nname: demo\ndescription: Demo\nargs: [topic]\n---\nTeach {{topic}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &runner{}
	svc := New(Config{
		Store:       modulesession.NewStore(t.TempDir(), nil),
		Bus:         event.NewBus(),
		Source:      source{},
		SkillsDir:   dir,
		SkillRunner: r,
		Now:         func() time.Time { return time.Unix(10, 0) },
	})
	sks, err := svc.ListSkills(false)
	if err != nil || len(sks) != 1 || sks[0].Name != "demo" {
		t.Fatalf("ListSkills = %#v, %v", sks, err)
	}
	if err := svc.RunSkill(context.Background(), RunSkillRequest{Name: "demo", Args: map[string]any{"topic": "math"}}); err != nil {
		t.Fatalf("RunSkill: %v", err)
	}
	if r.got.UserMsg != "Teach math\n" {
		t.Fatalf("skill prompt = %q", r.got.UserMsg)
	}
}

func TestToolCatalogAndEffectiveUseRuntimeSource(t *testing.T) {
	t.Parallel()
	svc := New(Config{Store: modulesession.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
	ts, err := svc.ToolCatalog(context.Background(), "s1")
	if err != nil || len(ts) != 1 || ts[0].Name != "sample" {
		t.Fatalf("ToolCatalog = %#v, %v", ts, err)
	}
	payload, err := svc.Dispatch(context.Background(), Request{Method: "tools.effective", Params: json.RawMessage(`{"session":"s1"}`)})
	if err != nil {
		t.Fatalf("tools.effective: %v", err)
	}
	effective := payload.([]ToolInfo)
	if len(effective) != 1 || effective[0].Name != "sample" {
		t.Fatalf("effective = %#v", effective)
	}
}

func TestCostSummaryReadsJSONLRecords(t *testing.T) {
	t.Parallel()
	costPath := filepath.Join(t.TempDir(), "cost.jsonl")
	body := strings.Join([]string{
		`{"session":"s1","input_tokens":10,"output_tokens":5,"cached_input_tokens":2,"cost_usd":0.25}`,
		`{"session":"s2","input_tokens":7,"output_tokens":3,"cached_input_tokens":1,"cost_usd":null}`,
	}, "\n") + "\n"
	if err := os.WriteFile(costPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := New(Config{CostPath: costPath})
	costs, err := svc.CostSummary("s1")
	if err != nil {
		t.Fatalf("CostSummary: %v", err)
	}
	if costs.Turns != 1 || costs.InputTokens != 10 || costs.CostUSD != 0.25 {
		t.Fatalf("costs = %#v", costs)
	}
	total, err := svc.CostSummary("")
	if err != nil {
		t.Fatalf("total CostSummary: %v", err)
	}
	if total.Turns != 2 || total.InputTokens != 17 || total.UnknownCostRecords != 1 {
		t.Fatalf("total costs = %#v", total)
	}
}

func TestSubscribeNormalizesAndFiltersEvents(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	svc := New(Config{Bus: bus})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := svc.Subscribe(ctx, "s1", event.TokenDelta)
	bus.Publish(event.Event{Kind: event.TokenDelta, Session: "other", At: time.Unix(1, 0), Payload: "skip"})
	bus.Publish(event.Event{Kind: event.TokenDelta, Session: "s1", At: time.Unix(2, 0), Payload: "keep"})
	select {
	case got := <-events:
		if got.Seq != 1 || got.Event != string(event.TokenDelta) || got.Session != "s1" || got.Payload != "keep" {
			t.Fatalf("event = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for normalized event")
	}
}

func TestGatewayTransportAppPreservesTransportCompatibility(t *testing.T) {
	t.Parallel()
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte("---\nname: demo\n---\nRun it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &runner{}
	svc := New(Config{
		Provider:    provider,
		Store:       modulesession.NewStore(t.TempDir(), nil),
		Bus:         event.NewBus(),
		Source:      source{},
		SkillsDir:   dir,
		SkillRunner: r,
	})
	app := TransportApp{Service: svc}
	if err := app.Submit(context.Background(), "s1", "hi"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := app.TriggerSkill(context.Background(), "demo", nil); err != nil {
		t.Fatalf("TriggerSkill: %v", err)
	}
	if r.got.SkillName != "demo" {
		t.Fatalf("triggered skill = %q", r.got.SkillName)
	}
}

type captureProvider struct {
	models []string
}

func (p *captureProvider) Chat(_ context.Context, req llm.Request) (<-chan llm.Chunk, error) {
	p.models = append(p.models, req.Model)
	ch := make(chan llm.Chunk, 2)
	ch <- llm.Chunk{TextDelta: "ok"}
	ch <- llm.Chunk{FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func TestSessionModelOverrideIsLocalToSession(t *testing.T) {
	t.Parallel()
	provider := &captureProvider{}
	svc := New(Config{
		Provider: provider,
		Store:    modulesession.NewStore(t.TempDir(), nil),
		Bus:      event.NewBus(),
		Source:   source{},
		Model:    "default-model",
	})
	if got := svc.CurrentModel("s1"); got != "default-model" {
		t.Fatalf("CurrentModel default = %q", got)
	}
	if err := svc.SetSessionModel("s1", "alt-model"); err != nil {
		t.Fatalf("SetSessionModel: %v", err)
	}
	if got := svc.CurrentModel("s1"); got != "alt-model" {
		t.Fatalf("CurrentModel s1 = %q", got)
	}
	if got := svc.CurrentModel("s2"); got != "default-model" {
		t.Fatalf("CurrentModel s2 = %q", got)
	}
	if _, err := svc.SubmitChat(context.Background(), ChatRequest{Session: "s1", Message: "hi"}); err != nil {
		t.Fatalf("SubmitChat s1: %v", err)
	}
	if _, err := svc.SubmitChat(context.Background(), ChatRequest{Session: "s2", Message: "hi"}); err != nil {
		t.Fatalf("SubmitChat s2: %v", err)
	}
	if got := strings.Join(provider.models, ","); got != "alt-model,default-model" {
		t.Fatalf("models = %s", got)
	}
}

type fakeCatalog struct {
	calls  int
	models []ModelInfo
	err    error
}

func (f *fakeCatalog) ListModels(context.Context) ([]ModelInfo, error) {
	f.calls++
	return append([]ModelInfo(nil), f.models...), f.err
}

func TestModelCatalogCacheTTLAndFlush(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	cat := &fakeCatalog{models: []ModelInfo{{ID: "m1"}, {ID: "m2"}}}
	svc := New(Config{ModelCatalog: cat, Now: func() time.Time { return now }})
	first, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels first: %v", err)
	}
	second, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels cached: %v", err)
	}
	if cat.calls != 1 || len(first) != 2 || len(second) != 2 {
		t.Fatalf("calls=%d first=%#v second=%#v", cat.calls, first, second)
	}
	now = now.Add(25 * time.Hour)
	cat.models = []ModelInfo{{ID: "m3"}}
	third, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels expired: %v", err)
	}
	if cat.calls != 2 || len(third) != 1 || third[0].ID != "m3" {
		t.Fatalf("calls=%d third=%#v", cat.calls, third)
	}
	svc.FlushModelCache()
	cat.models = []ModelInfo{{ID: "m4"}}
	fourth, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels flushed: %v", err)
	}
	if cat.calls != 3 || fourth[0].ID != "m4" {
		t.Fatalf("calls=%d fourth=%#v", cat.calls, fourth)
	}
}

func TestModelOperationsDispatch(t *testing.T) {
	t.Parallel()
	cat := &fakeCatalog{models: []ModelInfo{{ID: "m1"}}}
	svc := New(Config{Model: "default-model", ModelCatalog: cat})
	payload, err := svc.Dispatch(context.Background(), Request{Method: "models.current", Params: json.RawMessage(`{"session":"s1"}`)})
	if err != nil {
		t.Fatalf("models.current: %v", err)
	}
	if got := payload.(ModelState).Model; got != "default-model" {
		t.Fatalf("current = %q", got)
	}
	if _, err := svc.Dispatch(context.Background(), Request{Method: "models.use", Params: json.RawMessage(`{"session":"s1","model":"m1"}`)}); err != nil {
		t.Fatalf("models.use: %v", err)
	}
	if got := svc.CurrentModel("s1"); got != "m1" {
		t.Fatalf("s1 model = %q", got)
	}
	payload, err = svc.Dispatch(context.Background(), Request{Method: "models.list"})
	if err != nil {
		t.Fatalf("models.list: %v", err)
	}
	if got := payload.([]ModelInfo); len(got) != 1 || got[0].ID != "m1" {
		t.Fatalf("models = %#v", got)
	}
	if _, err := svc.Dispatch(context.Background(), Request{Method: "models.flush"}); err != nil {
		t.Fatalf("models.flush: %v", err)
	}
}

type fakeVoiceController struct {
	state VoiceState
	err   error
}

func (f *fakeVoiceController) State(context.Context, string) (VoiceState, error) {
	return f.state, f.err
}

func (f *fakeVoiceController) Update(_ context.Context, _ string, patch VoicePatch) (VoiceState, error) {
	if patch.STTEnabled != nil {
		f.state.STTEnabled = *patch.STTEnabled
	}
	if patch.TTSEnabled != nil {
		f.state.TTSEnabled = *patch.TTSEnabled
	}
	return f.state, f.err
}

type fakeRealtimeVoice struct {
	state RealtimeVoiceState
	err   error
}

func (f *fakeRealtimeVoice) Start(context.Context) (RealtimeVoiceState, error) {
	if f.err != nil {
		return f.state, f.err
	}
	f.state.Connected = true
	return f.state, nil
}

func (f *fakeRealtimeVoice) Stop(context.Context) (RealtimeVoiceState, error) {
	if f.err != nil {
		return f.state, f.err
	}
	f.state.Connected = false
	return f.state, nil
}

func (f *fakeRealtimeVoice) Status(context.Context) (RealtimeVoiceState, error) {
	return f.state, f.err
}

func TestVoiceAndXAIControlsDispatch(t *testing.T) {
	t.Parallel()
	vc := &fakeVoiceController{}
	rt := &fakeRealtimeVoice{state: RealtimeVoiceState{Provider: "xai", Model: "grok", SessionID: "voice-1"}}
	svc := New(Config{Voice: vc, RealtimeVoice: rt})
	if _, err := svc.Dispatch(context.Background(), Request{Method: "voice.update", Params: json.RawMessage(`{"session":"s1","stt_enabled":true}`)}); err != nil {
		t.Fatalf("voice.update: %v", err)
	}
	if !vc.state.STTEnabled {
		t.Fatalf("voice state = %#v", vc.state)
	}
	payload, err := svc.Dispatch(context.Background(), Request{Method: "xai.start"})
	if err != nil {
		t.Fatalf("xai.start: %v", err)
	}
	if got := payload.(RealtimeVoiceState); !got.Connected || got.Provider != "xai" {
		t.Fatalf("xai state = %#v", got)
	}
}

func TestVoiceControlsReturnStructuredUnavailable(t *testing.T) {
	t.Parallel()
	svc := New(Config{Voice: &fakeVoiceController{err: errors.New("voice unavailable")}})
	_, err := svc.Dispatch(context.Background(), Request{Method: "voice.update", Params: json.RawMessage(`{"stt_enabled":true}`)})
	if AsError(err).Code != CodeUnsupported {
		t.Fatalf("code = %v err=%v", AsError(err).Code, err)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
