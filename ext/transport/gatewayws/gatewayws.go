// Package gatewayws exposes the nanogo gateway service over OpenClaw-style
// websocket envelopes.
package gatewayws

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/coder/websocket"
	"github.com/tvmaly/nanogo/modules/gateway"
)

const ProtocolVersion = 1

type AuthConfig struct {
	Bearer              string `json:"-"`
	BearerEnv           string `json:"bearer_env,omitempty"`
	InsecureAllowNoAuth bool   `json:"insecure_allow_no_auth,omitempty"`
}

type Config struct {
	Addr string     `json:"addr,omitempty"`
	Path string     `json:"path,omitempty"`
	Auth AuthConfig `json:"auth,omitempty"`
}

type Server struct {
	cfg     Config
	service *gateway.Service
	mux     *http.ServeMux
	server  *http.Server
}

type Envelope struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload any             `json:"payload,omitempty"`
	Error   *gateway.Error  `json:"error,omitempty"`
	Event   string          `json:"event,omitempty"`
	Seq     int64           `json:"seq,omitempty"`
}

type ConnectRequest struct {
	MinProtocol int      `json:"minProtocol"`
	MaxProtocol int      `json:"maxProtocol"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes"`
	Client      string   `json:"client"`
	Auth        struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	} `json:"auth"`
}

func New(cfg Config, svc *gateway.Service) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8082"
	}
	if cfg.Path == "" {
		cfg.Path = "/gateway"
	}
	s := &Server{cfg: cfg, service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc(cfg.Path, s.handleWS)
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

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	ctx := r.Context()
	_, data, err := c.Read(ctx)
	if err != nil {
		return
	}
	var first Envelope
	if err := json.Unmarshal(data, &first); err != nil || first.Type != "req" || first.Method != "connect" {
		_ = write(ctx, c, Envelope{Type: "res", ID: first.ID, Error: gateway.E(gateway.CodeConnectionRequired, "first frame must be connect")})
		return
	}
	if ge := s.validateConnect(first.Params); ge != nil {
		_ = write(ctx, c, Envelope{Type: "res", ID: first.ID, Error: ge})
		return
	}
	if err := write(ctx, c, Envelope{Type: "res", ID: first.ID, OK: true, Payload: map[string]any{"protocol": ProtocolVersion}}); err != nil {
		return
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := s.service.Subscribe(subCtx, "")
	go func() {
		for ev := range events {
			_ = write(subCtx, c, Envelope{Type: "event", Event: ev.Event, Seq: ev.Seq, Payload: ev.Payload})
		}
	}()

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var req Envelope
		if err := json.Unmarshal(data, &req); err != nil || req.Type != "req" {
			_ = write(ctx, c, Envelope{Type: "res", ID: req.ID, Error: gateway.E(gateway.CodeInvalidRequest, "request envelope required")})
			continue
		}
		payload, err := s.service.Dispatch(ctx, gateway.Request{Method: req.Method, Params: req.Params})
		if err != nil {
			_ = write(ctx, c, Envelope{Type: "res", ID: req.ID, Error: gateway.AsError(err)})
			continue
		}
		_ = write(ctx, c, Envelope{Type: "res", ID: req.ID, OK: true, Payload: payload})
	}
}

func (s *Server) validateConnect(raw json.RawMessage) *gateway.Error {
	var req ConnectRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return gateway.E(gateway.CodeInvalidRequest, err.Error())
	}
	if req.MinProtocol > ProtocolVersion || req.MaxProtocol < ProtocolVersion {
		return gateway.E(gateway.CodeProtocolMismatch, "protocol v1 is required")
	}
	tok := s.bearer()
	if tok == "" && s.cfg.Auth.InsecureAllowNoAuth {
		return nil
	}
	if tok == "" {
		return gateway.E(gateway.CodeUnauthorized, "bearer token required")
	}
	if strings.ToLower(req.Auth.Type) != "bearer" || req.Auth.Token != tok {
		return gateway.E(gateway.CodeUnauthorized, "invalid bearer token")
	}
	return nil
}

func write(ctx context.Context, c *websocket.Conn, env Envelope) error {
	b, _ := json.Marshal(env)
	return c.Write(ctx, websocket.MessageText, b)
}
