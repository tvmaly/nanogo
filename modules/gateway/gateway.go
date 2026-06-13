// Package gateway provides the shared interface-facing service used by local
// and remote operator transports.
package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/help"
	"github.com/tvmaly/nanogo/modules/lesson"
	"github.com/tvmaly/nanogo/modules/media"
	"github.com/tvmaly/nanogo/modules/skills"
	"github.com/tvmaly/nanogo/modules/transport"
)

type ErrorCode string

const (
	CodeUnknownMethod      ErrorCode = "unknown_method"
	CodeUnsupported        ErrorCode = "unsupported"
	CodeInvalidRequest     ErrorCode = "invalid_request"
	CodeInternal           ErrorCode = "internal"
	CodeUnsupportedFeature ErrorCode = "unsupported_feature"
	CodeUnauthorized       ErrorCode = "unauthorized"
	CodeProtocolMismatch   ErrorCode = "protocol_mismatch"
	CodeConnectionRequired ErrorCode = "connection_required"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Retry   bool      `json:"retry"`
}

func (e *Error) Error() string { return e.Message }

func E(code ErrorCode, msg string) *Error { return &Error{Code: code, Message: msg} }

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var ge *Error
	if errors.As(err, &ge) {
		return ge
	}
	return &Error{Code: CodeInternal, Message: err.Error()}
}

type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Operation func(context.Context, json.RawMessage) (any, error)

type Registry struct {
	mu  sync.RWMutex
	ops map[string]Operation
}

func NewRegistry() *Registry { return &Registry{ops: map[string]Operation{}} }

func (r *Registry) Register(method string, op Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops[method] = op
}

func (r *Registry) Dispatch(ctx context.Context, req Request) (any, error) {
	r.mu.RLock()
	op := r.ops[req.Method]
	r.mu.RUnlock()
	if op == nil {
		if reserved(req.Method) {
			return nil, E(CodeUnsupported, "method "+req.Method+" is reserved for a future gateway extension")
		}
		return nil, E(CodeUnknownMethod, "unknown method "+req.Method)
	}
	return op(ctx, req.Params)
}

func reserved(method string) bool {
	for _, p := range []string{"approvals.", "heartbeats.", "memory.", "config.", "voice.", "adaptive.", "traces.", "help."} {
		if strings.HasPrefix(method, p) {
			return true
		}
	}
	return false
}

type SourceFactory func(context.Context, string) (tools.Source, error)

type ModelInfo struct {
	ID      string         `json:"id"`
	Name    string         `json:"name,omitempty"`
	Context int            `json:"context,omitempty"`
	Pricing map[string]any `json:"pricing,omitempty"`
}

type ModelCatalog interface {
	ListModels(context.Context) ([]ModelInfo, error)
}

type ModelState struct {
	Session string `json:"session,omitempty"`
	Model   string `json:"model"`
}

type VoiceState struct {
	Session    string `json:"session,omitempty"`
	STTEnabled bool   `json:"stt_enabled"`
	TTSEnabled bool   `json:"tts_enabled"`
}

type VoicePatch struct {
	STTEnabled *bool `json:"stt_enabled,omitempty"`
	TTSEnabled *bool `json:"tts_enabled,omitempty"`
}

type VoiceController interface {
	State(context.Context, string) (VoiceState, error)
	Update(context.Context, string, VoicePatch) (VoiceState, error)
}

type RealtimeVoiceState struct {
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Connected bool   `json:"connected"`
}

type RealtimeVoiceController interface {
	Start(context.Context) (RealtimeVoiceState, error)
	Stop(context.Context) (RealtimeVoiceState, error)
	Status(context.Context) (RealtimeVoiceState, error)
}

type Config struct {
	Provider      llm.Provider
	Store         session.Store
	Bus           event.Bus
	Source        tools.Source
	SourceFactory SourceFactory
	Model         string
	SkillsDir     string
	SkillRunner   skills.AgentRunner
	CostPath      string
	Now           func() time.Time
	ModelCatalog  ModelCatalog
	Voice         VoiceController
	RealtimeVoice RealtimeVoiceController
	Help          help.Service
	Browser       *browser.Service
	Lesson        *lesson.Service
	LessonBundles []lesson.Bundle
	Media         *media.Store
}

type Service struct {
	cfg      Config
	registry *Registry

	mu           sync.Mutex
	sessions     map[string]session.Session
	models       map[string]string
	modelCache   []ModelInfo
	modelCacheAt time.Time
}

func New(cfg Config) *Service {
	if cfg.Bus == nil {
		cfg.Bus = event.NewBus()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{cfg: cfg, registry: NewRegistry(), sessions: map[string]session.Session{}, models: map[string]string{}}
	s.registerBuiltins()
	s.registerBrowserOps()
	s.registerLessonOps()
	s.registerMediaOps()
	return s
}

func (s *Service) Registry() *Registry { return s.registry }
func (s *Service) Dispatch(ctx context.Context, req Request) (any, error) {
	return s.registry.Dispatch(ctx, req)
}

type Status struct {
	OK       bool     `json:"ok"`
	Model    string   `json:"model,omitempty"`
	Methods  []string `json:"methods"`
	Sessions int      `json:"sessions"`
}

func (s *Service) Status() Status {
	s.registry.mu.RLock()
	methods := make([]string, 0, len(s.registry.ops))
	for m := range s.registry.ops {
		methods = append(methods, m)
	}
	s.registry.mu.RUnlock()
	sort.Strings(methods)
	s.mu.Lock()
	n := len(s.sessions)
	s.mu.Unlock()
	return Status{OK: true, Model: s.cfg.Model, Methods: methods, Sessions: n}
}

func (s *Service) CurrentModel(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID != "" {
		if model := s.models[sessionID]; model != "" {
			return model
		}
	}
	return s.cfg.Model
}

func (s *Service) SetSessionModel(sessionID, model string) error {
	if strings.TrimSpace(model) == "" {
		return E(CodeInvalidRequest, "model is required")
	}
	if sessionID == "" {
		sessionID = "tui"
	}
	s.mu.Lock()
	s.models[sessionID] = model
	s.mu.Unlock()
	return nil
}

func (s *Service) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if s.cfg.ModelCatalog == nil {
		return nil, E(CodeUnsupported, "model catalog is not configured")
	}
	now := s.cfg.Now()
	s.mu.Lock()
	if !s.modelCacheAt.IsZero() && now.Sub(s.modelCacheAt) < 24*time.Hour {
		out := append([]ModelInfo(nil), s.modelCache...)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	models, err := s.cfg.ModelCatalog.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.modelCache = append([]ModelInfo(nil), models...)
	s.modelCacheAt = now
	out := append([]ModelInfo(nil), s.modelCache...)
	s.mu.Unlock()
	return out, nil
}

func (s *Service) FlushModelCache() {
	s.mu.Lock()
	s.modelCache = nil
	s.modelCacheAt = time.Time{}
	s.mu.Unlock()
}

type ChatRequest struct {
	Session string `json:"session,omitempty"`
	Message string `json:"message"`
}

type ChatResponse struct {
	Session string        `json:"session"`
	Text    string        `json:"text"`
	Events  []EventRecord `json:"events,omitempty"`
}

func (s *Service) SubmitChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(req.Message) == "" {
		return ChatResponse{}, E(CodeInvalidRequest, "message is required")
	}
	if req.Session == "" {
		req.Session = fmt.Sprintf("gw-%d", s.cfg.Now().UnixNano())
	}
	sess, err := s.session(req.Session)
	if err != nil {
		return ChatResponse{}, err
	}
	sess.Append(llm.Message{Role: "user", Content: req.Message})
	src, err := s.source(ctx, req.Session)
	if err != nil {
		return ChatResponse{}, err
	}

	evtCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sub := s.cfg.Bus.Subscribe(evtCtx, event.TokenDelta, event.TurnCompleted, event.Error)
	done := make(chan error, 1)
	go func() {
		done <- agent.NewLoop(agent.Config{
			Provider:   s.cfg.Provider,
			Source:     src,
			Session:    sess,
			Bus:        s.cfg.Bus,
			Model:      s.CurrentModel(req.Session),
			SourceName: "gateway",
		}).Run(ctx)
	}()

	var b strings.Builder
	var records []EventRecord
	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return ChatResponse{}, ctx.Err()
			}
			if e.Session != "" && e.Session != req.Session {
				continue
			}
			rec := NormalizeEvent(0, e)
			records = append(records, rec)
			switch e.Kind {
			case event.TokenDelta:
				if delta, ok := e.Payload.(string); ok {
					b.WriteString(delta)
				}
			case event.TurnCompleted:
				if err := <-done; err != nil {
					return ChatResponse{}, err
				}
				return ChatResponse{Session: req.Session, Text: b.String(), Events: records}, nil
			case event.Error:
				_ = <-done
				return ChatResponse{}, E(CodeInternal, fmt.Sprint(e.Payload))
			}
		case err := <-done:
			if err != nil {
				return ChatResponse{}, err
			}
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		}
	}
}

type StreamEvent struct {
	Kind  string `json:"kind"`
	Delta string `json:"delta,omitempty"`
	Text  string `json:"text,omitempty"`
	Err   error  `json:"-"`
}

func (s *Service) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, E(CodeInvalidRequest, "message is required")
	}
	if req.Session == "" {
		req.Session = fmt.Sprintf("gw-%d", s.cfg.Now().UnixNano())
	}
	sess, err := s.session(req.Session)
	if err != nil {
		return nil, err
	}
	src, err := s.source(ctx, req.Session)
	if err != nil {
		return nil, err
	}
	sess.Append(llm.Message{Role: "user", Content: req.Message})
	sub := s.cfg.Bus.Subscribe(ctx, event.TokenDelta, event.TurnCompleted, event.Error)
	out := make(chan StreamEvent, 16)
	go func() {
		defer close(out)
		done := make(chan error, 1)
		go func() {
			done <- agent.NewLoop(agent.Config{Provider: s.cfg.Provider, Source: src, Session: sess, Bus: s.cfg.Bus, Model: s.CurrentModel(req.Session), SourceName: "gateway"}).Run(ctx)
		}()
		var b strings.Builder
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
					if delta, ok := e.Payload.(string); ok {
						b.WriteString(delta)
						out <- StreamEvent{Kind: "delta", Delta: delta}
					}
				case event.TurnCompleted:
					if err := <-done; err != nil {
						out <- StreamEvent{Kind: "error", Err: err}
						return
					}
					out <- StreamEvent{Kind: "done", Text: b.String()}
					return
				case event.Error:
					_ = <-done
					out <- StreamEvent{Kind: "error", Err: E(CodeInternal, fmt.Sprint(e.Payload))}
					return
				}
			case err := <-done:
				if err != nil {
					out <- StreamEvent{Kind: "error", Err: err}
				}
				return
			case <-ctx.Done():
				out <- StreamEvent{Kind: "error", Err: ctx.Err()}
				return
			}
		}
	}()
	return out, nil
}

type SessionInfo struct {
	ID       string         `json:"id"`
	Status   session.Status `json:"status"`
	Messages []llm.Message  `json:"messages,omitempty"`
}

func (s *Service) CreateSession(id string) (SessionInfo, error) {
	if id == "" {
		id = fmt.Sprintf("gw-%d", s.cfg.Now().UnixNano())
	}
	sess, err := s.session(id)
	if err != nil {
		return SessionInfo{}, err
	}
	return SessionInfo{ID: sess.ID(), Status: sess.GetStatus()}, nil
}

func (s *Service) GetSession(id string, includeMessages bool) (SessionInfo, error) {
	sess, err := s.session(id)
	if err != nil {
		return SessionInfo{}, err
	}
	info := SessionInfo{ID: sess.ID(), Status: sess.GetStatus()}
	if includeMessages {
		info.Messages = sess.Messages()
	}
	return info, nil
}

func (s *Service) ListSessions() []SessionInfo {
	s.mu.Lock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Strings(ids)
	out := make([]SessionInfo, 0, len(ids))
	for _, id := range ids {
		info, err := s.GetSession(id, false)
		if err == nil {
			out = append(out, info)
		}
	}
	return out
}

func (s *Service) DeleteSession(id string) error {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	if s.cfg.Store == nil {
		return nil
	}
	if err := s.cfg.Store.Delete(id); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type SkillInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Args        []string `json:"args,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

func (s *Service) ListSkills(all bool) ([]SkillInfo, error) {
	if s.cfg.SkillsDir == "" {
		return nil, nil
	}
	found, err := skills.Discover(s.cfg.SkillsDir, nil)
	if err != nil {
		return nil, err
	}
	loader := skills.NewLoader(found)
	list := loader.UserFacing()
	if all {
		list = loader.All()
	}
	out := make([]SkillInfo, 0, len(list))
	for _, sk := range list {
		out = append(out, SkillInfo{Name: sk.Name, Description: sk.Description, Kind: sk.Kind, Args: sk.Args, Tools: sk.Tools})
	}
	return out, nil
}

type RunSkillRequest struct {
	Name    string         `json:"name"`
	Args    map[string]any `json:"args,omitempty"`
	Session string         `json:"session,omitempty"`
}

func (s *Service) RunSkill(ctx context.Context, req RunSkillRequest) error {
	if req.Name == "" {
		return E(CodeInvalidRequest, "skill name is required")
	}
	if s.cfg.SkillRunner == nil {
		return E(CodeUnsupported, "skill runner is not configured")
	}
	found, err := skills.Discover(s.cfg.SkillsDir, nil)
	if err != nil {
		return err
	}
	return skills.NewDispatcher(skills.NewLoader(found), s.cfg.SkillRunner).Fire(ctx, skills.Trigger{
		Skill: req.Name, Source: skills.SourceCLI, Args: req.Args, Session: req.Session,
	})
}

type ToolInfo struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

func (s *Service) ToolCatalog(ctx context.Context, sessionID string) ([]ToolInfo, error) {
	src, err := s.source(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	ts, err := src.Tools(ctx, tools.TurnInfo{Session: sessionID})
	if err != nil {
		return nil, err
	}
	out := make([]ToolInfo, 0, len(ts))
	for _, t := range ts {
		out = append(out, ToolInfo{Name: t.Name(), Schema: t.Schema()})
	}
	return out, nil
}

type CostSummary struct {
	Turns              int     `json:"turns"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CachedInputTokens  int     `json:"cached_input_tokens"`
	CostUSD            float64 `json:"cost_usd,omitempty"`
	UnknownCostRecords int     `json:"unknown_cost_records,omitempty"`
}

func (s *Service) CostSummary(sessionID string) (CostSummary, error) {
	if s.cfg.CostPath == "" {
		return CostSummary{}, nil
	}
	f, err := os.Open(s.cfg.CostPath)
	if os.IsNotExist(err) {
		return CostSummary{}, nil
	}
	if err != nil {
		return CostSummary{}, err
	}
	defer f.Close()
	var out CostSummary
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r struct {
			Session           string   `json:"session"`
			InputTokens       int      `json:"input_tokens"`
			OutputTokens      int      `json:"output_tokens"`
			CachedInputTokens int      `json:"cached_input_tokens"`
			CostUSD           *float64 `json:"cost_usd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return CostSummary{}, err
		}
		if sessionID != "" && r.Session != sessionID {
			continue
		}
		out.Turns++
		out.InputTokens += r.InputTokens
		out.OutputTokens += r.OutputTokens
		out.CachedInputTokens += r.CachedInputTokens
		if r.CostUSD != nil {
			out.CostUSD += *r.CostUSD
		} else {
			out.UnknownCostRecords++
		}
	}
	return out, sc.Err()
}

type EventRecord struct {
	Seq     int64     `json:"seq"`
	Event   string    `json:"event"`
	Session string    `json:"session,omitempty"`
	At      time.Time `json:"at"`
	Payload any       `json:"payload,omitempty"`
}

func NormalizeEvent(seq int64, e event.Event) EventRecord {
	return EventRecord{Seq: seq, Event: string(e.Kind), Session: e.Session, At: e.At, Payload: e.Payload}
}

func (s *Service) Subscribe(ctx context.Context, sessionID string, kinds ...event.Kind) <-chan EventRecord {
	if len(kinds) == 0 {
		kinds = []event.Kind{event.TurnStarted, event.TokenDelta, event.ToolCallStarted, event.ToolCallResult, event.TurnCompleted, event.AskUser, event.Error}
	}
	raw := s.cfg.Bus.Subscribe(ctx, kinds...)
	out := make(chan EventRecord, 32)
	go func() {
		defer close(out)
		var seq int64
		for e := range raw {
			if sessionID != "" && e.Session != "" && e.Session != sessionID {
				continue
			}
			seq++
			out <- NormalizeEvent(seq, e)
		}
	}()
	return out
}

func (s *Service) source(ctx context.Context, sessionID string) (tools.Source, error) {
	if s.cfg.SourceFactory != nil {
		return s.cfg.SourceFactory(ctx, sessionID)
	}
	if s.cfg.Source != nil {
		return s.cfg.Source, nil
	}
	return nil, E(CodeUnsupported, "tool source is not configured")
}

func (s *Service) session(id string) (session.Session, error) {
	if id == "" {
		return nil, E(CodeInvalidRequest, "session id is required")
	}
	s.mu.Lock()
	if sess := s.sessions[id]; sess != nil {
		s.mu.Unlock()
		return sess, nil
	}
	s.mu.Unlock()
	if s.cfg.Store == nil {
		return nil, E(CodeUnsupported, "session store is not configured")
	}
	sess, err := s.cfg.Store.Load(id)
	if err != nil {
		sess, err = s.cfg.Store.Create(id)
	}
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, E(CodeInvalidRequest, err.Error())
	}
	return v, nil
}

func (s *Service) helpService() (help.Service, error) {
	if s.cfg.Help == nil {
		return nil, E(CodeUnsupported, "help service is not configured")
	}
	return s.cfg.Help, nil
}

func (s *Service) registerBuiltins() {
	s.registerStatusDiagnostics()
	s.registerChatSessionsEvents()
	s.registerSkillsTools()
	s.registerCostsModels()
	s.registerVoiceRealtime()
	s.registerHelp()
}

func (s *Service) registerStatusDiagnostics() {
	s.registry.Register("status", func(context.Context, json.RawMessage) (any, error) { return s.Status(), nil })
}

func (s *Service) registerChatSessionsEvents() {
	s.registry.Register("agent", func(ctx context.Context, raw json.RawMessage) (any, error) {
		req, err := decode[ChatRequest](raw)
		if err != nil {
			return nil, err
		}
		return s.SubmitChat(ctx, req)
	})
	s.registry.Register("sessions.create", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.CreateSession(req.ID)
	})
	s.registry.Register("sessions.get", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return s.GetSession(req.ID, false)
	})
	s.registry.Register("sessions.messages", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return s.GetSession(req.ID, true)
	})
	s.registry.Register("sessions.list", func(context.Context, json.RawMessage) (any, error) {
		return s.ListSessions(), nil
	})
	s.registry.Register("sessions.delete", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return map[string]bool{"deleted": true}, s.DeleteSession(req.ID)
	})
	s.registry.Register("events.subscribe", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		return map[string]any{"subscribed": true, "session": req.Session}, nil
	})
}

func (s *Service) registerSkillsTools() {
	s.registry.Register("skills.list", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			All bool `json:"all"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.ListSkills(req.All)
	})
	s.registry.Register("skills.run", func(ctx context.Context, raw json.RawMessage) (any, error) {
		req, err := decode[RunSkillRequest](raw)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.RunSkill(ctx, req)
	})
	s.registry.Register("tools.catalog", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.ToolCatalog(ctx, req.Session)
	})
	s.registry.Register("tools.effective", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.ToolCatalog(ctx, req.Session)
	})
}

func (s *Service) registerCostsModels() {
	s.registry.Register("costs.summary", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.CostSummary(req.Session)
	})
	s.registry.Register("models.current", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		return ModelState{Session: req.Session, Model: s.CurrentModel(req.Session)}, nil
	})
	s.registry.Register("models.use", func(_ context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session string `json:"session"`
			Model   string `json:"model"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		if err := s.SetSessionModel(req.Session, req.Model); err != nil {
			return nil, err
		}
		return ModelState{Session: req.Session, Model: s.CurrentModel(req.Session)}, nil
	})
	s.registry.Register("models.list", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.ListModels(ctx)
	})
	s.registry.Register("models.flush", func(context.Context, json.RawMessage) (any, error) {
		s.FlushModelCache()
		return map[string]bool{"flushed": true}, nil
	})
}

func (s *Service) registerVoiceRealtime() {
	s.registry.Register("voice.state", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if s.cfg.Voice == nil {
			return nil, E(CodeUnsupported, "voice controller is not configured")
		}
		var req struct {
			Session string `json:"session"`
		}
		_ = json.Unmarshal(raw, &req)
		state, err := s.cfg.Voice.State(ctx, req.Session)
		if err != nil {
			return nil, E(CodeUnsupported, err.Error())
		}
		return state, nil
	})
	s.registry.Register("voice.update", func(ctx context.Context, raw json.RawMessage) (any, error) {
		if s.cfg.Voice == nil {
			return nil, E(CodeUnsupported, "voice controller is not configured")
		}
		var req struct {
			Session string `json:"session"`
			VoicePatch
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		state, err := s.cfg.Voice.Update(ctx, req.Session, req.VoicePatch)
		if err != nil {
			return nil, E(CodeUnsupported, err.Error())
		}
		return state, nil
	})
	s.registry.Register("xai.start", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if s.cfg.RealtimeVoice == nil {
			return nil, E(CodeUnsupported, "xai realtime voice controller is not configured")
		}
		return s.cfg.RealtimeVoice.Start(ctx)
	})
	s.registry.Register("xai.stop", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if s.cfg.RealtimeVoice == nil {
			return nil, E(CodeUnsupported, "xai realtime voice controller is not configured")
		}
		return s.cfg.RealtimeVoice.Stop(ctx)
	})
	s.registry.Register("xai.status", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if s.cfg.RealtimeVoice == nil {
			return nil, E(CodeUnsupported, "xai realtime voice controller is not configured")
		}
		return s.cfg.RealtimeVoice.Status(ctx)
	})
}

func (s *Service) registerHelp() {
	s.registry.Register("help.search", func(ctx context.Context, raw json.RawMessage) (any, error) {
		h, err := s.helpService()
		if err != nil {
			return nil, err
		}
		req, err := decode[help.SearchRequest](raw)
		if err != nil {
			return nil, err
		}
		return h.Search(ctx, req)
	})
	s.registry.Register("help.topic", func(ctx context.Context, raw json.RawMessage) (any, error) {
		h, err := s.helpService()
		if err != nil {
			return nil, err
		}
		req, err := decode[help.TopicRequest](raw)
		if err != nil {
			return nil, err
		}
		resp, err := h.Topic(ctx, req)
		if errors.Is(err, help.ErrNotFound) {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return resp, err
	})
	s.registry.Register("help.suggest", func(ctx context.Context, raw json.RawMessage) (any, error) {
		h, err := s.helpService()
		if err != nil {
			return nil, err
		}
		req, err := decode[help.SuggestRequest](raw)
		if err != nil {
			return nil, err
		}
		return h.Suggest(ctx, req)
	})
	s.registry.Register("help.render", func(ctx context.Context, raw json.RawMessage) (any, error) {
		h, err := s.helpService()
		if err != nil {
			return nil, err
		}
		req, err := decode[help.RenderRequest](raw)
		if err != nil {
			return nil, err
		}
		resp, err := h.Render(ctx, req)
		if errors.Is(err, help.ErrNotFound) {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return resp, err
	})
	s.registry.Register("help.validate", func(ctx context.Context, _ json.RawMessage) (any, error) {
		h, err := s.helpService()
		if err != nil {
			return nil, err
		}
		return h.Validate(ctx, help.ValidateRequest{})
	})
}

type TransportApp struct{ Service *Service }

var _ transport.App = TransportApp{}

func (a TransportApp) Submit(ctx context.Context, sessionID, message string) error {
	_, err := a.Service.SubmitChat(ctx, ChatRequest{Session: sessionID, Message: message})
	return err
}

func (a TransportApp) Resume(context.Context, string, string) error {
	return E(CodeUnsupported, "resume is not implemented by gateway transport adapter")
}

func (a TransportApp) TriggerSkill(ctx context.Context, name string, args map[string]any) error {
	return a.Service.RunSkill(ctx, RunSkillRequest{Name: name, Args: args})
}
