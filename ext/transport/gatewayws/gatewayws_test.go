package gatewayws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/help"
	helpfake "github.com/tvmaly/nanogo/modules/help/fake"
	"github.com/tvmaly/nanogo/modules/skills"
)

type source struct{}

func (source) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return []tools.Tool{sampleTool{}}, nil
}

type sampleTool struct{}

func (sampleTool) Name() string { return "sample" }
func (sampleTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"sample"}}`)
}
func (sampleTool) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

type skillRunner struct{}

func (skillRunner) RunSkill(context.Context, skills.RunSkillOpts) (string, error) { return "ran", nil }

func testGatewayServer(t *testing.T) (*httptest.Server, string, *gateway.Service, event.Bus) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte("---\nname: demo\n---\nRun it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	costPath := filepath.Join(t.TempDir(), "cost.jsonl")
	if err := os.WriteFile(costPath, []byte(`{"session":"s1","input_tokens":1,"output_tokens":2,"cached_input_tokens":0,"cost_usd":0.01}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	bus := event.NewBus()
	svc := gateway.New(gateway.Config{
		Provider:    provider,
		Store:       session.NewStore(t.TempDir(), nil),
		Bus:         bus,
		Source:      source{},
		SkillsDir:   dir,
		SkillRunner: skillRunner{},
		CostPath:    costPath,
		Help: &helpfake.Service{
			SearchResp: help.SearchResponse{Hits: []help.SearchHit{{ID: "tui.slash_commands", Title: "TUI Slash Commands", Summary: "TUI help", Kind: "command", Snippet: "TUI help"}}},
			TopicResp:  help.TopicResponse{Topic: help.Topic{TopicMeta: help.TopicMeta{ID: "tui.slash_commands", Title: "TUI Slash Commands"}, SourcePaths: []string{"ext/transport/tui"}, Body: "body"}},
		},
	})
	s := New(Config{Path: "/gateway", Auth: AuthConfig{Bearer: "secret"}}, svc)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts, "ws" + strings.TrimPrefix(ts.URL, "http") + "/gateway", svc, bus
}

func TestHelpOperationsWorkOverWebSocket(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	ctx := context.Background()
	c := dialAndConnect(t, url)
	defer c.CloseNow()
	if err := write(ctx, c, Envelope{Type: "req", ID: "help-search", Method: "help.search", Params: json.RawMessage(`{"query":"tui"}`)}); err != nil {
		t.Fatal(err)
	}
	got := readResponseID(t, c, "help-search")
	if !got.OK || !strings.Contains(fmt.Sprint(got.Payload), "tui.slash_commands") {
		t.Fatalf("help.search = %#v", got)
	}
	if err := write(ctx, c, Envelope{Type: "req", ID: "help-topic", Method: "help.topic", Params: json.RawMessage(`{"id":"tui.slash_commands"}`)}); err != nil {
		t.Fatal(err)
	}
	got = readResponseID(t, c, "help-topic")
	if !got.OK || !strings.Contains(fmt.Sprint(got.Payload), "TUI Slash Commands") {
		t.Fatalf("help.topic = %#v", got)
	}
}

func TestConnectRequiredAndAuth(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := write(ctx, c, Envelope{Type: "req", ID: "1", Method: "status"}); err != nil {
		t.Fatal(err)
	}
	var got Envelope
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Error == nil || got.Error.Code != gateway.CodeConnectionRequired {
		t.Fatalf("got %#v", got)
	}
}

func TestConnectNegotiatesProtocolV1(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	c := dialAndConnect(t, url)
	defer c.CloseNow()
}

func TestProtocolMismatchRejected(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	params := json.RawMessage(`{"minProtocol":2,"maxProtocol":3,"role":"operator","scopes":["*"],"client":"test","auth":{"type":"bearer","token":"secret"}}`)
	if err := write(ctx, c, Envelope{Type: "req", ID: "c", Method: "connect", Params: params}); err != nil {
		t.Fatal(err)
	}
	got := readEnvelope(t, c)
	if got.Error == nil || got.Error.Code != gateway.CodeProtocolMismatch {
		t.Fatalf("got %#v", got)
	}
}

func TestWebSocketBearerAuthEnforced(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	for _, auth := range []string{
		`{"type":"bearer","token":"wrong"}`,
		`{"type":"basic","token":"secret"}`,
		`{}`,
	} {
		ctx := context.Background()
		c, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		params := json.RawMessage(`{"minProtocol":1,"maxProtocol":1,"role":"operator","scopes":["*"],"client":"test","auth":` + auth + `}`)
		if err := write(ctx, c, Envelope{Type: "req", ID: "c", Method: "connect", Params: params}); err != nil {
			t.Fatal(err)
		}
		got := readEnvelope(t, c)
		_ = c.CloseNow()
		if got.Error == nil || got.Error.Code != gateway.CodeUnauthorized {
			t.Fatalf("auth %s got %#v", auth, got)
		}
	}
}

func TestReqResRoundTripAndUnsupportedNamespace(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	ctx := context.Background()
	c := dialAndConnect(t, url)
	defer c.CloseNow()
	var got Envelope
	if err := write(ctx, c, Envelope{Type: "req", ID: "1", Method: "status"}); err != nil {
		t.Fatal(err)
	}
	got = readResponseID(t, c, "1")
	if !got.OK || got.ID != "1" {
		t.Fatalf("status = %#v", got)
	}
	if err := write(ctx, c, Envelope{Type: "req", ID: "2", Method: "voice.start"}); err != nil {
		t.Fatal(err)
	}
	got = readResponseID(t, c, "2")
	if got.Error == nil || got.Error.Code != gateway.CodeUnsupported {
		t.Fatalf("unsupported = %#v", got)
	}
}

func TestGatewayMethodsWorkOverWebSocket(t *testing.T) {
	_, url, _, _ := testGatewayServer(t)
	ctx := context.Background()
	c := dialAndConnect(t, url)
	defer c.CloseNow()
	cases := []Envelope{
		{Type: "req", ID: "agent", Method: "agent", Params: json.RawMessage(`{"session":"s1","message":"hi"}`)},
		{Type: "req", ID: "skill", Method: "skills.run", Params: json.RawMessage(`{"name":"demo"}`)},
		{Type: "req", ID: "tools", Method: "tools.catalog", Params: json.RawMessage(`{"session":"s1"}`)},
		{Type: "req", ID: "costs", Method: "costs.summary", Params: json.RawMessage(`{"session":"s1"}`)},
	}
	for _, req := range cases {
		if err := write(ctx, c, req); err != nil {
			t.Fatal(err)
		}
		got := readResponseID(t, c, req.ID)
		if !got.OK {
			t.Fatalf("%s = %#v", req.Method, got)
		}
	}
}

func TestBusEventsAreForwardedAsEventEnvelopes(t *testing.T) {
	_, url, _, bus := testGatewayServer(t)
	c := dialAndConnect(t, url)
	defer c.CloseNow()
	bus.Publish(event.Event{Kind: event.TokenDelta, Session: "s1", At: time.Now(), Payload: "hello"})
	deadline := time.After(time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for event envelope")
		default:
			got := readEnvelope(t, c)
			if got.Type != "event" {
				continue
			}
			if got.Event != string(event.TokenDelta) || got.Seq != 1 || got.Payload != "hello" {
				t.Fatalf("event = %#v", got)
			}
			return
		}
	}
}

func dialAndConnect(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"minProtocol":1,"maxProtocol":1,"role":"operator","scopes":["*"],"client":"test","auth":{"type":"bearer","token":"secret"}}`)
	if err := write(ctx, c, Envelope{Type: "req", ID: "c", Method: "connect", Params: params}); err != nil {
		t.Fatal(err)
	}
	got := readEnvelope(t, c)
	if !got.OK {
		t.Fatalf("connect = %#v", got)
	}
	raw, ok := got.Payload.(map[string]any)
	if !ok || raw["protocol"].(float64) != float64(ProtocolVersion) {
		t.Fatalf("connect payload = %#v", got.Payload)
	}
	return c
}

func readEnvelope(t *testing.T, c *websocket.Conn) Envelope {
	t.Helper()
	_, data, err := c.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got Envelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func readResponseID(t *testing.T, c *websocket.Conn, id string) Envelope {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for response %s", id)
		default:
			got := readEnvelope(t, c)
			if got.Type == "res" && got.ID == id {
				return got
			}
		}
	}
}
