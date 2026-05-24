// Package openaiapi exposes a small OpenAI-compatible HTTP surface over the
// gateway service.
package openaiapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/modules/gateway"
)

type AuthConfig struct {
	Bearer              string `json:"-"`
	BearerEnv           string `json:"bearer_env,omitempty"`
	InsecureAllowNoAuth bool   `json:"insecure_allow_no_auth,omitempty"`
}

type Config struct {
	Addr string     `json:"addr,omitempty"`
	Auth AuthConfig `json:"auth,omitempty"`
}

type Server struct {
	cfg     Config
	service *gateway.Service
	mux     *http.ServeMux
	server  *http.Server
}

func New(cfg Config, svc *gateway.Service) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8081"
	}
	s := &Server{cfg: cfg, service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/models", s.withAuth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.withAuth(s.handleChatCompletions))
	mux.HandleFunc("POST /nanogo/v1/operations", s.withAuth(s.handleOperation))
	mux.HandleFunc("GET /nanogo/v1/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("GET /nanogo/v1/skills", s.withAuth(s.handleSkills))
	mux.HandleFunc("GET /nanogo/v1/tools", s.withAuth(s.handleTools))
	mux.HandleFunc("GET /nanogo/v1/costs", s.withAuth(s.handleCosts))
	s.mux = mux
	s.server = &http.Server{Addr: cfg.Addr, Handler: mux}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }
func (s *Server) Start() error                                     { return s.server.ListenAndServe() }
func (s *Server) Stop(ctx context.Context) error                   { return s.server.Shutdown(ctx) }

func (s *Server) bearer() string {
	if s.cfg.Auth.Bearer != "" {
		return s.cfg.Auth.Bearer
	}
	if s.cfg.Auth.BearerEnv != "" {
		return os.Getenv(s.cfg.Auth.BearerEnv)
	}
	return ""
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := s.bearer()
		if tok == "" && s.cfg.Auth.InsecureAllowNoAuth {
			next(w, r)
			return
		}
		if tok == "" {
			writeGatewayError(w, http.StatusUnauthorized, gateway.E(gateway.CodeUnauthorized, "bearer token required"))
			return
		}
		if got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); got != tok || got == r.Header.Get("Authorization") {
			writeGatewayError(w, http.StatusUnauthorized, gateway.E(gateway.CodeUnauthorized, "invalid bearer token"))
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	status := s.service.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id":       first(status.Model, "nanogo-gateway"),
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "nanogo",
		}},
	})
}

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []chatMessage   `json:"messages"`
	Stream      bool            `json:"stream"`
	Tools       json.RawMessage `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	Modalities  []string        `json:"modalities,omitempty"`
	Session     string          `json:"session,omitempty"`
	UnknownSink map[string]any  `json:"-"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeInvalidRequest, err.Error()))
		return
	}
	data, _ := json.Marshal(raw)
	var req chatRequest
	if err := json.Unmarshal(data, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeInvalidRequest, err.Error()))
		return
	}
	if len(req.Tools) > 0 || len(req.ToolChoice) > 0 {
		writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeUnsupportedFeature, "client-supplied tools are not executed by the OpenAI-compatible adapter"))
		return
	}
	for _, m := range req.Modalities {
		if m != "text" {
			writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeUnsupportedFeature, "only text chat completions are supported"))
			return
		}
	}
	prompt, err := promptFromMessages(req.Messages)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeUnsupportedFeature, err.Error()))
		return
	}
	if req.Stream {
		s.streamChat(w, r, req, prompt)
		return
	}
	resp, err := s.service.SubmitChat(r.Context(), gateway.ChatRequest{Session: req.Session, Message: prompt})
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, gateway.AsError(err))
		return
	}
	writeJSON(w, http.StatusOK, completion(req.Model, resp.Text, false))
}

func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, req chatRequest, prompt string) {
	ch, err := s.service.StreamChat(r.Context(), gateway.ChatRequest{Session: req.Session, Message: prompt})
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, gateway.AsError(err))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	for ev := range ch {
		if ev.Err != nil {
			writeSSE(w, map[string]any{"error": gateway.AsError(ev.Err)})
			break
		}
		if ev.Kind == "delta" {
			writeSSE(w, completion(req.Model, ev.Delta, true))
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func promptFromMessages(msgs []chatMessage) (string, error) {
	if len(msgs) == 0 {
		return "", fmt.Errorf("messages are required")
	}
	var parts []string
	for _, m := range msgs {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			parts = append(parts, s)
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal(m.Content, &arr); err == nil {
			for _, item := range arr {
				if item["type"] != "text" {
					return "", fmt.Errorf("only text message content is supported")
				}
				if text, _ := item["text"].(string); text != "" {
					parts = append(parts, text)
				}
			}
			continue
		}
		return "", fmt.Errorf("message content must be text")
	}
	return strings.Join(parts, "\n"), nil
}

func completion(model, content string, stream bool) map[string]any {
	if model == "" {
		model = "nanogo-gateway"
	}
	msgKey := "message"
	msg := map[string]any{"role": "assistant", "content": content}
	if stream {
		msgKey = "delta"
		msg = map[string]any{"content": content}
	}
	return map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  map[bool]string{true: "chat.completion.chunk", false: "chat.completion"}[stream],
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			msgKey:  msg,
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	var req gateway.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, gateway.E(gateway.CodeInvalidRequest, err.Error()))
		return
	}
	payload, err := s.service.Dispatch(r.Context(), req)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, gateway.AsError(err))
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Status())
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "true"
	out, err := s.service.ListSkills(all)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, gateway.AsError(err))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	out, err := s.service.ToolCatalog(r.Context(), r.URL.Query().Get("session"))
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, gateway.AsError(err))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	out, err := s.service.CostSummary(r.URL.Query().Get("session"))
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, gateway.AsError(err))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func writeGatewayError(w http.ResponseWriter, status int, err *gateway.Error) {
	writeJSON(w, status, map[string]any{"error": err})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
