package rest_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport/fake"
	"github.com/tvmaly/nanogo/ext/transport/rest"
)

func newTestTransport(t *testing.T, token string) (*rest.Transport, *fake.App, event.Bus) {
	t.Helper()
	bus := event.NewBus()
	app := &fake.App{Bus: bus}
	tr := rest.New(rest.Config{Addr: ":0", Auth: rest.AuthConfig{Bearer: token}}, bus, app)
	return tr, app, bus
}

// TEST-5.5 — healthcheck
func TestHealthz(t *testing.T) {
	t.Parallel()
	tr, _, _ := newTestTransport(t, "")
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("want 'ok', got %q", rec.Body.String())
	}
}

// TEST-5.4 — auth enforced
func TestAuthMissing(t *testing.T) {
	t.Parallel()
	tr, _, _ := newTestTransport(t, "secret")
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat", strings.NewReader(`{"session":"s1","message":"hi"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestAuthWrong(t *testing.T) {
	t.Parallel()
	tr, _, _ := newTestTransport(t, "secret")
	req := httptest.NewRequest("POST", "/v1/chat", strings.NewReader(`{"session":"s1","message":"hi"}`))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestAuthBearerEnv(t *testing.T) {
	t.Setenv("NANOGO_TEST_TOKEN", "env-secret")
	bus := event.NewBus()
	app := &fake.App{Bus: bus}
	tr := rest.New(rest.Config{Addr: ":0", Auth: rest.AuthConfig{BearerEnv: "NANOGO_TEST_TOKEN"}}, bus, app)

	req := httptest.NewRequest("POST", "/v1/chat", strings.NewReader(`{"session":"s1","message":"hi"}`))
	req.Header.Set("Authorization", "Bearer env-secret")
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	os.Unsetenv("NANOGO_TEST_TOKEN")
	rec = httptest.NewRecorder()
	tr.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unset env token should require auth, got %d", rec.Code)
	}
}

func TestAuthCorrectHealthz(t *testing.T) {
	t.Parallel()
	tr, _, _ := newTestTransport(t, "secret")
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz should not require auth, got %d", rec.Code)
	}
}

// TEST-5.1 — POST /v1/chat returns SSE stream ending with event: done
func TestChatSSE(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := &fake.App{Bus: bus}
	tr := rest.New(rest.Config{Addr: ":0"}, bus, app)

	body := bytes.NewBufferString(`{"session":"sse-test","message":"hello"}`)
	req := httptest.NewRequest("POST", "/v1/chat", body)
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("want text/event-stream, got %q", ct)
	}

	var sawDone bool
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		line := sc.Text()
		if line == "event: done" {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("SSE stream did not end with 'event: done'")
	}
}

// TEST-5.2 — POST /v1/skills/{name}/trigger returns 200 with session ID
func TestSkillTrigger(t *testing.T) {
	t.Parallel()
	tr, app, _ := newTestTransport(t, "")
	body := bytes.NewBufferString(`{"args":{"env":"dev","service":"api"}}`)
	req := httptest.NewRequest("POST", "/v1/skills/deploy-service/trigger", body)
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp["session"] == "" {
		t.Fatal("response missing session field")
	}
	if len(app.Triggers) != 1 || app.Triggers[0].Name != "deploy-service" {
		t.Fatalf("expected TriggerSkill called with deploy-service, got %+v", app.Triggers)
	}
}

// TEST-5.3 — POST /v1/sessions/{id}/resume returns 200
func TestResume(t *testing.T) {
	t.Parallel()
	tr, app, _ := newTestTransport(t, "")
	body := bytes.NewBufferString(`{"answer":"dev"}`)
	req := httptest.NewRequest("POST", "/v1/sessions/sess-1/resume", body)
	rec := httptest.NewRecorder()
	tr.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if len(app.Resumes) != 1 || app.Resumes[0].Session != "sess-1" || app.Resumes[0].Answer != "dev" {
		t.Fatalf("unexpected resumes: %+v", app.Resumes)
	}
}
