package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	inFlight int
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
	if len(s.byID)+s.inFlight >= s.policy.MaxSessions {
		s.mu.Unlock()
		return Session{}, PolicyDenied("max_sessions_exceeded", "browser max sessions exceeded")
	}
	s.inFlight++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
	}()

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
	if sess.LessonEventNonce == "" {
		sess.LessonEventNonce = newNonce()
	}

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
	if err := s.ensureSession(req.SessionID); err != nil {
		return err
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
	if err := s.ensureSession(req.SessionID); err != nil {
		return PageState{}, err
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
	if err := s.ensureSession(req.SessionID); err != nil {
		return Snapshot{}, err
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
	if err := s.ensureSession(req.SessionID); err != nil {
		return TextResult{}, err
	}
	s.touch(req.SessionID)
	return s.controller.Text(ctx, req)
}

func (s *Service) Screenshot(ctx context.Context, req ScreenshotRequest) (Artifact, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return Artifact{}, err
	}
	if resolved, err := s.policy.resolveArtifactPath(req.Path); err != nil {
		return Artifact{}, err
	} else if resolved != "" {
		req.Path = resolved
	}
	s.touch(req.SessionID)
	return s.controller.Screenshot(ctx, req)
}

func (s *Service) PDF(ctx context.Context, req PDFRequest) (Artifact, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return Artifact{}, err
	}
	if resolved, err := s.policy.resolveArtifactPath(req.Path); err != nil {
		return Artifact{}, err
	} else if resolved != "" {
		req.Path = resolved
	}
	s.touch(req.SessionID)
	return s.controller.PDF(ctx, req)
}

func (s *Service) Act(ctx context.Context, req ActionRequest) (ActionResult, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return ActionResult{}, err
	}
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
	if err := s.ensureSession(req.SessionID); err != nil {
		return EvalResult{}, err
	}
	if !s.policy.AllowEval {
		return EvalResult{}, PolicyDenied("eval_not_allowed", "browser eval is disabled")
	}
	s.touch(req.SessionID)
	res, err := s.controller.Eval(ctx, req)
	if err != nil {
		return EvalResult{}, err
	}
	res.NetworkIsolation = false
	res.NetworkIsolationAdvisory = "JavaScript-initiated network requests are not filtered by Nanogo browser domain policy."
	return res, nil
}

func (s *Service) Wait(ctx context.Context, req WaitRequest) (WaitResult, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return WaitResult{}, err
	}
	s.touch(req.SessionID)
	return s.controller.Wait(ctx, req)
}

func (s *Service) Tabs(ctx context.Context, req TabsRequest) (TabsResult, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return TabsResult{}, err
	}
	s.touch(req.SessionID)
	return s.controller.Tabs(ctx, req)
}

func (s *Service) MediaSeek(ctx context.Context, req MediaSeekRequest) (MediaSeekResult, error) {
	if err := s.ensureSession(req.SessionID); err != nil {
		return MediaSeekResult{}, err
	}
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
	if ev.SessionID != "" {
		s.mu.Lock()
		sess, ok := s.byID[ev.SessionID]
		s.mu.Unlock()
		if !ok {
			return staleSessionError(ev.SessionID)
		}
		if sess.LessonEventNonce != "" && ev.Nonce != sess.LessonEventNonce {
			return E(CodeNotAuthorized, "lesson event nonce is invalid")
		}
	}
	s.publish(ev.SessionID, EventLessonEvent, ev)
	if ev.Kind == "completion" && ev.SessionID != "" {
		s.publish(ev.SessionID, EventLessonCompleted, ev)
		return s.Close(ctx, CloseRequest{SessionID: ev.SessionID, CloseSession: true, Reason: "lesson_completed"})
	}
	return nil
}

func (s *Service) ensureSession(id SessionID) error {
	if id == "" {
		return Invalid("session_id is required")
	}
	s.mu.Lock()
	_, ok := s.byID[id]
	s.mu.Unlock()
	if !ok {
		return staleSessionError(id)
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
	sessions := make([]registrySession, 0, len(s.byID))
	for _, sess := range s.byID {
		sessions = append(sessions, registrySession{
			SessionID:  sess.ID,
			Driver:     firstNonEmpty(sess.Metadata["driver"], s.policy.Driver),
			StartedAt:  sess.CreatedAt,
			LastUsedAt: sess.LastUsedAt,
			TTLSeconds: s.policy.WithDefaults().SessionTTLSeconds,
			Headed:     sess.Headed,
			Metadata:   safeMetadata(sess.Metadata),
		})
	}
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.registry), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"version": 1, "sessions": sessions}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.registry, data, 0600)
}

type registrySession struct {
	SessionID  SessionID         `json:"session_id"`
	Driver     string            `json:"driver,omitempty"`
	StartedAt  time.Time         `json:"started_at,omitempty"`
	LastUsedAt time.Time         `json:"last_used_at,omitempty"`
	TTLSeconds int               `json:"ttl_seconds,omitempty"`
	Headed     bool              `json:"headed"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func safeMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range in {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "profile") || strings.Contains(lower, "cookie") ||
			strings.Contains(lower, "storage") || strings.Contains(lower, "authorization") ||
			strings.Contains(lower, "screenshot") {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func staleSessionError(id SessionID) error {
	return E(CodeNotFound, fmt.Sprintf("browser session %q was not found; start a new session with browser_session_start", id))
}

func newNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func mergeNonEmpty(a, b []string) []string {
	if len(b) == 0 {
		return append([]string(nil), a...)
	}
	out := append([]string(nil), a...)
	out = append(out, b...)
	return out
}
