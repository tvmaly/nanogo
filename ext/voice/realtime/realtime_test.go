package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
)

func TestProviderAdapterInterface(t *testing.T) {
	var _ ProviderAdapter = staticAdapter{}
	var _ RealtimeConn = staticConn{}
}

func TestRealtimeEventRoundtrip(t *testing.T) {
	in := Event{
		Type:        EventResponseAudioDelta,
		AudioBase64: "AAEC",
		Raw:         json.RawMessage(`{"type":"provider.event"}`),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Event
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Type != in.Type || out.AudioBase64 != in.AudioBase64 || string(out.Raw) != string(in.Raw) {
		t.Fatalf("roundtrip = %#v", out)
	}
}

func TestWebSocketDialerRequest(t *testing.T) {
	var gotAuth string
	srv := newWebSocketTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "ok")
	}))
	defer srv.Close()

	ctx := context.Background()
	conn, err := DialWebSocket(ctx, "ws"+srv.URL[len("http"):]+"?model=test", map[string]string{
		"Authorization": "Bearer test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q", gotAuth)
	}
}

func TestWebSocketTextFrames(t *testing.T) {
	srv := newWebSocketTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "ok")
		typ, b, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if typ != websocket.MessageText {
			t.Errorf("type = %v", typ)
		}
		if err := conn.Write(r.Context(), websocket.MessageText, b); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	conn, err := DialWebSocket(ctx, "ws"+srv.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.Send(ctx, Event{Type: EventResponseCreate}); err != nil {
		t.Fatal(err)
	}
	got, err := conn.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != EventResponseCreate {
		t.Fatalf("type = %q", got.Type)
	}
}

type staticAdapter struct{}

func (staticAdapter) Name() string { return "static" }
func (staticAdapter) Connect(context.Context, ProviderConfig) (RealtimeConn, error) {
	return staticConn{}, nil
}

type staticConn struct{}

func (staticConn) Send(context.Context, Event) error      { return nil }
func (staticConn) Receive(context.Context) (Event, error) { return Event{}, nil }
func (staticConn) Close() error                           { return nil }

func newWebSocketTestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("local listener unavailable in sandbox: %v", r)
			}
		}()
		srv = httptest.NewServer(h)
	}()
	if srv == nil {
		t.Skip("local listener unavailable in sandbox")
	}
	if srv.URL == "" {
		t.Fatal(fmt.Errorf("test server missing URL"))
	}
	return srv
}
