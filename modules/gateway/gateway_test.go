package gateway

import (
	"context"
	"encoding/json"
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
		Store:    session.NewStore(t.TempDir(), nil),
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
		Store:    session.NewStore(t.TempDir(), nil),
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
	svc := New(Config{Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
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
		Store:       session.NewStore(t.TempDir(), nil),
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
	svc := New(Config{Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
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
		Store:       session.NewStore(t.TempDir(), nil),
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

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
