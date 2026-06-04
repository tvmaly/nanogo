package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

type Service struct {
	controller Controller
	policy     Policy
	bus        event.Bus
	now        func() time.Time

	mu       sync.Mutex
	byID     map[SessionID]Session
	byName   map[string]SessionID
	registry string
}

type ServiceConfig struct {
	Controller Controller
	Policy     Policy
	Bus        event.Bus
	Now        func() time.Time
	Registry   string
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Controller == nil {
		return nil, fmt.Errorf("browser service: controller is required")
	}
	if cfg.Bus == nil {
		cfg.Bus = event.NewBus()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	p := cfg.Policy.WithDefaults()
	return &Service{
		controller: cfg.Controller,
		policy:     p,
		bus:        cfg.Bus,
		now:        cfg.Now,
		byID:       map[SessionID]Session{},
		byName:     map[string]SessionID{},
		registry:   cfg.Registry,
	}, nil
}

func (s *Service) Policy() Policy { return s.policy }

func (s *Service) HasSession() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID) > 0
}

func (s *Service) Health(ctx context.Context) (Health, error) {
	return s.controller.Health(ctx)
}

func (s *Service) Start(ctx context.Context, req StartRequest) (Session, error) {
	if !validSessionName(req.SessionName) {
		return Session{}, Invalid("session_name must contain only letters, numbers, dot, dash, or underscore")
	}
	s.mu.Lock()
	if req.SessionName != "" {
		if id := s.byName[req.SessionName]; id != "" {
			existing := s.byID[id]
			existing.LastUsedAt = s.now()
			s.byID[id] = existing
			s.mu.Unlock()
			return existing, nil
		}
	}
	if len(s.byID) >= s.policy.MaxSessions {
		s.mu.Unlock()
		return Session{}, PolicyDenied("max_sessions_exceeded", "browser max sessions exceeded")
	}
	s.mu.Unlock()

	req.AllowedDomains = mergeNonEmpty(s.policy.AllowedDomains, req.AllowedDomains)
	req.FileRoots = mergeNonEmpty(s.policy.AllowFileRoots, req.FileRoots)
	sess, err := s.controller.Start(ctx, req)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	sess.LastUsedAt = now
	sess.Name = req.SessionName
	sess.Headed = req.Headed

	s.mu.Lock()
	s.byID[sess.ID] = sess
	if sess.Name != "" {
		s.byName[sess.Name] = sess.ID
	}
	s.mu.Unlock()
	s.publish(sess.ID, EventSessionStarted, sess)
	_ = s.writeRegistry()
	return sess, nil
}

func (s *Service) Connect(ctx context.Context, req ConnectRequest) (Session, error) {
	if err := s.policy.checkEndpoint(req.Endpoint); err != nil {
		return Session{}, err
	}
	return s.controller.Connect(ctx, req)
}

func (s *Service) Close(ctx context.Context, req CloseRequest) error {
	if req.SessionID == "" {
		return Invalid("session_id is required")
	}
	if err := s.controller.Close(ctx, req); err != nil {
		return err
	}
	s.mu.Lock()
	sess := s.byID[req.SessionID]
	delete(s.byID, req.SessionID)
	if sess.Name != "" {
		delete(s.byName, sess.Name)
	}
	s.mu.Unlock()
	payload := map[string]any{"session_id": req.SessionID, "reason": req.Reason}
	s.publish(req.SessionID, EventSessionClosed, payload)
	_ = s.writeRegistry()
	return nil
}

func (s *Service) Cleanup(ctx context.Context) error {
	cutoff := s.now().Add(-s.policy.TTL())
	var stale []SessionID
	s.mu.Lock()
	for id, sess := range s.byID {
		if sess.LastUsedAt.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	s.mu.Unlock()
	for _, id := range stale {
		if err := s.Close(ctx, CloseRequest{SessionID: id, CloseSession: true, Reason: "ttl_expired"}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	ids := make([]SessionID, 0, len(s.byID))
	for id := range s.byID {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		if err := s.Close(ctx, CloseRequest{SessionID: id, CloseSession: true, Reason: "shutdown"}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Navigate(ctx context.Context, req NavigateRequest) (PageState, error) {
	if req.SessionID == "" {
		return PageState{}, Invalid("session_id is required")
	}
	if err := s.policy.checkURL(req.URL); err != nil {
		return PageState{}, err
	}
	s.touch(req.SessionID)
	page, err := s.controller.Navigate(ctx, req)
	if err != nil {
		return PageState{}, err
	}
	s.publish(req.SessionID, EventPageNavigated, page)
	if req.WaitUntil != "none" {
		s.publish(req.SessionID, EventPageLoaded, page)
	}
	return page, nil
}

func (s *Service) Snapshot(ctx context.Context, req SnapshotRequest) (Snapshot, error) {
	if req.SessionID == "" {
		return Snapshot{}, Invalid("session_id is required")
	}
	if req.MaxDepth <= 0 {
		req.MaxDepth = s.policy.SnapshotMaxDepth
	}
	if req.MaxOutputBytes <= 0 {
		req.MaxOutputBytes = s.policy.SnapshotMaxOutputByte
	}
	s.touch(req.SessionID)
	return s.controller.Snapshot(ctx, req)
}

func (s *Service) Text(ctx context.Context, req TextRequest) (TextResult, error) {
	s.touch(req.SessionID)
	return s.controller.Text(ctx, req)
}

func (s *Service) Screenshot(ctx context.Context, req ScreenshotRequest) (Artifact, error) {
	s.touch(req.SessionID)
	return s.controller.Screenshot(ctx, req)
}

func (s *Service) PDF(ctx context.Context, req PDFRequest) (Artifact, error) {
	s.touch(req.SessionID)
	return s.controller.PDF(ctx, req)
}

func (s *Service) Act(ctx context.Context, req ActionRequest) (ActionResult, error) {
	if req.Kind == ActionUpload && !s.policy.AllowUploads {
		return ActionResult{}, PolicyDenied("upload_not_allowed", "browser file upload is disabled")
	}
	s.touch(req.SessionID)
	res, err := s.controller.Act(ctx, req)
	if err != nil {
		return ActionResult{}, err
	}
	s.publish(req.SessionID, EventActionDone, res)
	return res, nil
}

func (s *Service) Eval(ctx context.Context, req EvalRequest) (EvalResult, error) {
	if !s.policy.AllowEval {
		return EvalResult{}, PolicyDenied("eval_not_allowed", "browser eval is disabled")
	}
	s.touch(req.SessionID)
	return s.controller.Eval(ctx, req)
}

func (s *Service) Wait(ctx context.Context, req WaitRequest) (WaitResult, error) {
	s.touch(req.SessionID)
	return s.controller.Wait(ctx, req)
}

func (s *Service) Tabs(ctx context.Context, req TabsRequest) (TabsResult, error) {
	s.touch(req.SessionID)
	return s.controller.Tabs(ctx, req)
}

func (s *Service) MediaSeek(ctx context.Context, req MediaSeekRequest) (MediaSeekResult, error) {
	if req.Strategy == "" {
		req.Strategy = "auto"
	}
	switch req.Strategy {
	case "auto", "youtube_iframe_api", "html5_video", "postmessage":
	default:
		return MediaSeekResult{}, E(CodeUnsupportedStrategy, "browser media seek strategy is unsupported")
	}
	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 3000
	}
	s.touch(req.SessionID)
	res, err := s.controller.MediaSeek(ctx, req)
	if err != nil {
		return MediaSeekResult{}, err
	}
	s.publish(req.SessionID, EventMediaSeek, res)
	return res, nil
}

func (s *Service) RecordLessonEvent(ctx context.Context, ev LessonEvent) error {
	switch ev.Kind {
	case "completion", "quiz_answer", "progress", "error":
	default:
		return Invalid("lesson event kind is not allowed")
	}
	s.publish(ev.SessionID, EventLessonEvent, ev)
	if ev.Kind == "completion" && ev.SessionID != "" {
		return s.Close(ctx, CloseRequest{SessionID: ev.SessionID, CloseSession: true, Reason: "lesson_completed"})
	}
	return nil
}

func (s *Service) touch(id SessionID) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byID[id]; ok {
		sess.LastUsedAt = s.now()
		s.byID[id] = sess
	}
}

func (s *Service) publish(id SessionID, kind string, payload any) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(event.Event{Kind: event.Kind(kind), Session: string(id), At: s.now(), Payload: payload})
}

func (s *Service) writeRegistry() error {
	if s.registry == "" {
		return nil
	}
	s.mu.Lock()
	sessions := make([]Session, 0, len(s.byID))
	for _, sess := range s.byID {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.registry), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"sessions": sessions}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.registry, data, 0600)
}

func mergeNonEmpty(a, b []string) []string {
	if len(b) == 0 {
		return append([]string(nil), a...)
	}
	out := append([]string(nil), a...)
	out = append(out, b...)
	return out
}
