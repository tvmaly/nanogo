package otel_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/ext/obs/otel"
)

// collector accumulates OTLP/HTTP export bodies for inspection.
type collector struct {
	mu   sync.Mutex
	reqs [][]byte
}

func (c *collector) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, body)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (c *collector) bodies() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.reqs))
	copy(out, c.reqs)
	return out
}

func TestOtelSpansExported(t *testing.T) {
	t.Parallel()
	col := &collector{}
	srv := httptest.NewServer(col.handler())
	defer srv.Close()

	tr, err := otel.New(otel.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("otel.New: %v", err)
	}
	defer tr.Shutdown(context.Background())

	// Simulate the three span hierarchy: turn → llm.call → tool.call
	ctx := context.Background()
	turnCtx := tr.StartSpan(ctx, "turn")
	llmCtx := tr.StartSpan(turnCtx, "llm.call")
	toolCtx := tr.StartSpan(llmCtx, "tool.call")
	tr.EndSpan(toolCtx)
	tr.EndSpan(llmCtx)
	tr.EndSpan(turnCtx)

	// Flush and give the exporter time to send.
	tr.Flush(context.Background())
	time.Sleep(200 * time.Millisecond)

	bodies := col.bodies()
	if len(bodies) == 0 {
		t.Fatal("no OTLP export received by collector")
	}

	// Verify the export contains span data (JSON or protobuf; we use JSON exporter).
	combined := string(bodies[0])
	for _, want := range []string{"turn", "llm.call", "tool.call"} {
		if !containsString(combined, want) {
			t.Errorf("expected span name %q in export payload", want)
		}
	}
}

func TestOtelRecordEvent(t *testing.T) {
	t.Parallel()
	col := &collector{}
	srv := httptest.NewServer(col.handler())
	defer srv.Close()

	tr, err := otel.New(otel.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("otel.New: %v", err)
	}
	defer tr.Shutdown(context.Background())

	e := event.Event{
		Kind:    event.TurnCompleted,
		Session: "s1",
		Turn:    1,
		At:      time.Now(),
	}
	tr.RecordEvent(context.Background(), e)
	tr.Flush(context.Background())
	time.Sleep(200 * time.Millisecond)

	bodies := col.bodies()
	if len(bodies) == 0 {
		t.Fatal("no OTLP export received after RecordEvent")
	}
	combined := string(bodies[0])
	if !containsString(combined, "turn.completed") {
		t.Errorf("expected event kind in span name, got: %s", combined)
	}
}

// containsString does a simple substring check; also handles JSON-encoded strings.
func containsString(haystack, needle string) bool {
	if len(haystack) == 0 {
		return false
	}
	// Direct substring
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	// JSON-encoded needle (e.g. `"turn"`)
	enc, _ := json.Marshal(needle)
	encStr := string(enc)
	for i := 0; i <= len(haystack)-len(encStr); i++ {
		if haystack[i:i+len(encStr)] == encStr {
			return true
		}
	}
	return false
}
