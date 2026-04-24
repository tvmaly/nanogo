// Package rest implements an HTTP + SSE transport for the agent.
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport"
)

func init() {
	transport.Register("rest", func(cfg json.RawMessage, bus event.Bus, app transport.App) (transport.Transport, error) {
		var c Config
		if err := json.Unmarshal(cfg, &c); err != nil {
			return nil, err
		}
		if c.Addr == "" {
			c.Addr = ":8080"
		}
		return New(c, bus, app), nil
	})
}

// AuthConfig holds auth settings.
type AuthConfig struct {
	BearerEnv string `json:"bearer_env"`
	Bearer    string `json:"-"` // set directly in tests
}

// Config holds REST transport configuration.
type Config struct {
	Addr string     `json:"addr"`
	Auth AuthConfig `json:"auth"`
}

// Transport is the REST/SSE transport.
type Transport struct {
	cfg    Config
	bus    event.Bus
	app    transport.App
	server *http.Server
	mux    *http.ServeMux
}

// New constructs a REST Transport.
func New(cfg Config, bus event.Bus, app transport.App) *Transport {
	t := &Transport{cfg: cfg, bus: bus, app: app}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", t.handleHealthz)
	mux.HandleFunc("POST /v1/chat", t.withAuth(t.handleChat))
	mux.HandleFunc("POST /v1/skills/{name}/trigger", t.withAuth(t.handleSkillTrigger))
	mux.HandleFunc("POST /v1/sessions/{session}/resume", t.withAuth(t.handleResume))
	t.mux = mux
	t.server = &http.Server{Addr: cfg.Addr, Handler: mux}
	return t
}

// ServeHTTP allows using the Transport directly with httptest.
func (t *Transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mux.ServeHTTP(w, r)
}

func (t *Transport) Name() string { return "rest" }

// Start begins listening. Blocks until the server stops.
func (t *Transport) Start(ctx context.Context, app transport.App) error {
	if app != nil {
		t.app = app
	}
	return t.server.ListenAndServe()
}

// Stop gracefully shuts down the HTTP server.
func (t *Transport) Stop(ctx context.Context) error {
	return t.server.Shutdown(ctx)
}

func (t *Transport) bearer() string {
	if t.cfg.Auth.Bearer != "" {
		return t.cfg.Auth.Bearer
	}
	return ""
}

func (t *Transport) withAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := t.bearer()
		if tok != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != tok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		h(w, r)
	}
}

func (t *Transport) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, "ok")
}

type chatReq struct {
	Session string `json:"session"`
	Message string `json:"message"`
}

func (t *Transport) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Session == "" {
		req.Session = fmt.Sprintf("rest-%d", time.Now().UnixNano())
	}

	ctx := r.Context()
	sub := t.bus.Subscribe(ctx, event.TokenDelta, event.TurnCompleted, event.Error)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Session-Id", req.Session)
	flusher, _ := w.(http.Flusher)

	if err := t.app.Submit(ctx, req.Session, req.Message); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return
			}
			if e.Session != "" && e.Session != req.Session {
				continue
			}
			switch e.Kind {
			case event.TokenDelta:
				if s, ok := e.Payload.(string); ok {
					fmt.Fprintf(w, "data: %s\n\n", s)
					if flusher != nil {
						flusher.Flush()
					}
				}
			case event.TurnCompleted:
				fmt.Fprintf(w, "event: done\ndata: \n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			case event.Error:
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", e.Payload)
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

type skillTriggerReq struct {
	Args map[string]any `json:"args"`
}

func (t *Transport) handleSkillTrigger(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req skillTriggerReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	sessionID := fmt.Sprintf("skill-%s-%d", name, time.Now().UnixNano())
	if err := t.app.TriggerSkill(r.Context(), name, req.Args); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"session": sessionID, "status": "ok"})
}

type resumeReq struct {
	Answer string `json:"answer"`
}

func (t *Transport) handleResume(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session")
	var req resumeReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := t.app.Resume(r.Context(), sessionID, req.Answer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
