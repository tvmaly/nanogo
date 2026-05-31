package openaiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/help"
	helpfake "github.com/tvmaly/nanogo/modules/help/fake"
	modulesession "github.com/tvmaly/nanogo/modules/session"
)

type testSource struct{}

func (testSource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return []tools.Tool{testTool{}}, nil
}

type testTool struct{}

func (testTool) Name() string { return "sample" }
func (testTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"sample"}}`)
}
func (testTool) Call(context.Context, json.RawMessage) (string, error) { return "ok", nil }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{
		Provider: provider, Store: modulesession.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: testSource{}, Model: "m",
		Help: &helpfake.Service{SearchResp: help.SearchResponse{Hits: []help.SearchHit{{ID: "skills.markdown", Title: "Skills", Summary: "Skills help", Kind: "guide", Snippet: "Skills help"}}}},
	})
	return New(Config{Auth: AuthConfig{Bearer: "secret"}}, svc)
}

func authed(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	return req
}

func TestHealthzDoesNotRequireAuth(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestBearerAuthEnforced(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), string(gateway.CodeUnauthorized)) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInsecureAllowNoAuthAllowsProtectedEndpoints(t *testing.T) {
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{Provider: provider, Store: modulesession.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: testSource{}, Model: "m"})
	s := New(Config{Auth: AuthConfig{InsecureAllowNoAuth: true}}, svc)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestModelsReturnsOpenAIStyleList(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodGet, "/v1/models", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Object != "list" || len(got.Data) != 1 || got.Data[0].ID != "m" {
		t.Fatalf("models = %#v", got)
	}
}

func TestChatCompletionsNonStream(t *testing.T) {
	s := newTestServer(t)
	body := `{"model":"m","messages":[{"role":"user","content":"hi"}],"unknown":true}`
	req := authed(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Choices[0].Message.Content != "ok" {
		t.Fatalf("content = %q", got.Choices[0].Message.Content)
	}
}

func TestChatCompletionsAcceptsUnknownFields(t *testing.T) {
	s := newTestServer(t)
	body := `{"model":"m","metadata":{"trace":"x"},"messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/v1/chat/completions", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChatCompletionsStream(t *testing.T) {
	s := newTestServer(t)
	req := authed(http.MethodPost, "/v1/chat/completions", `{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") || !strings.Contains(rec.Body.String(), "chat.completion.chunk") {
		t.Fatalf("bad sse body: %s", rec.Body.String())
	}
}

func TestUnsupportedModalitiesReturnStructuredError(t *testing.T) {
	s := newTestServer(t)
	body := `{"model":"m","modalities":["audio"],"messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/v1/chat/completions", body))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), string(gateway.CodeUnsupportedFeature)) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body = `{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}}]}]}`
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/v1/chat/completions", body))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), string(gateway.CodeUnsupportedFeature)) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUnsupportedToolsAndAuth(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m","tools":[{}],"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), string(gateway.CodeUnsupportedFeature)) {
		t.Fatalf("unsupported status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNanogoControlEndpointsUseGatewayService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte("---\nname: demo\ndescription: Demo\n---\nRun it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	costPath := filepath.Join(t.TempDir(), "cost.jsonl")
	if err := os.WriteFile(costPath, []byte(`{"session":"s1","input_tokens":1,"output_tokens":2,"cached_input_tokens":0,"cost_usd":0.01}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{
		Provider:  provider,
		Store:     modulesession.NewStore(t.TempDir(), nil),
		Bus:       event.NewBus(),
		Source:    testSource{},
		Model:     "m",
		SkillsDir: dir,
		CostPath:  costPath,
	})
	s := New(Config{Auth: AuthConfig{Bearer: "secret"}}, svc)
	for _, path := range []string{"/nanogo/v1/status", "/nanogo/v1/skills", "/nanogo/v1/tools", "/nanogo/v1/costs"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, authed(http.MethodGet, path, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/nanogo/v1/operations", `{"method":"status"}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("operations status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOperationsEndpointDispatchesHelpWithoutChangingChatPath(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/nanogo/v1/operations", `{"method":"help.search","params":{"query":"skills"}}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "skills.markdown") {
		t.Fatalf("help operation status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodGet, "/nanogo/help", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("dedicated help route status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, authed(http.MethodPost, "/v1/chat/completions", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "skills.markdown") {
		t.Fatalf("chat status=%d body=%s", rec.Code, rec.Body.String())
	}
}
