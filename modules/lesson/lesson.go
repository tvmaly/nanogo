// Package lesson owns deterministic browser micro-lesson session state.
package lesson

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SchemaSession    = "lesson.session.v1"
	SchemaTurn       = "lesson.turn.v1"
	SchemaProgress   = "lesson.progress.v1"
	SchemaReviewItem = "lesson.review_item.v1"
	SchemaNextAction = "lesson.next_action.v1"
)

type ObjectiveType string
type EvidenceKind string
type CompletionRule string

const (
	ObjectiveSurface       ObjectiveType = "surface"
	ObjectiveDeep          ObjectiveType = "deep"
	ObjectiveTransfer      ObjectiveType = "transfer"
	ObjectivePhysicalSkill ObjectiveType = "physical_skill"
	ObjectiveMixed         ObjectiveType = "mixed"

	EvidencePhysicalPerformance EvidenceKind = "physical_performance"
	EvidenceStudentReasoning    EvidenceKind = "student_reasoning"
	EvidenceReflection          EvidenceKind = "reflection"
	EvidenceTransfer            EvidenceKind = "transfer"
	EvidenceParentConfirmation  EvidenceKind = "parent_confirmation"

	CompletionPhysicalPass        CompletionRule = "physical_pass"
	CompletionAllRequiredEvidence CompletionRule = "all_required_evidence"
	CompletionParentConfirmed     CompletionRule = "parent_confirmed"
)

type Config struct {
	Root  string
	Now   func() time.Time
	Nonce func() string
}

type Service struct {
	root     string
	now      func() time.Time
	nonce    func() string
	sessions map[string]Session
}

type Bundle struct {
	ID           string
	Title        string
	ChildID      string
	Approved     bool
	Promoted     bool
	Assigned     bool
	MicroLessons []MicroLesson
}

type MicroLesson struct {
	ID               string
	Requires         []string
	ObjectiveType    ObjectiveType
	LearningEvidence LearningEvidence
}

type LearningEvidence struct {
	Requires       []EvidenceKind
	CompletionRule CompletionRule
}

type Session struct {
	SchemaVersion  string `json:"schema_version"`
	ID             string `json:"lesson_session_id"`
	Nonce          string `json:"nonce"`
	ChildID        string `json:"child_id"`
	LessonID       string `json:"lesson_id"`
	CurrentMicroID string `json:"current_micro_lesson_id"`
}

type Event struct {
	Type          string         `json:"type"`
	Nonce         string         `json:"nonce"`
	ChildID       string         `json:"child_id,omitempty"`
	AttemptID     string         `json:"attempt_id,omitempty"`
	ActorRole     string         `json:"actor_role,omitempty"`
	MicroLessonID string         `json:"micro_lesson_id,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

type TurnRecord struct {
	SchemaVersion string         `json:"schema_version"`
	Timestamp     time.Time      `json:"timestamp"`
	ChildID       string         `json:"child_id"`
	LessonSession string         `json:"lesson_session_id"`
	LessonID      string         `json:"lesson_id"`
	MicroLessonID string         `json:"micro_lesson_id"`
	AttemptID     string         `json:"attempt_id,omitempty"`
	ActorRole     string         `json:"actor_role"`
	EventType     string         `json:"event_type"`
	Data          map[string]any `json:"data,omitempty"`
}

type Verdict struct {
	VisualPass       bool
	Reasoning        bool
	Reflection       bool
	Transfer         bool
	ParentConfirmed  bool
	TrustRampPermits bool
}

type AdvanceResult struct {
	Advanced             bool   `json:"advanced"`
	CurrentMicroLessonID string `json:"current_micro_lesson_id"`
	RemediationRef       string `json:"remediation_ref,omitempty"`
}

type ReviewItem struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	ChildID       string    `json:"child_id"`
	LessonID      string    `json:"lesson_id"`
	MicroLessonID string    `json:"micro_lesson_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type ProgressRow struct {
	SchemaVersion string    `json:"schema_version"`
	ChildID       string    `json:"child_id"`
	LessonID      string    `json:"lesson_id"`
	MicroLessonID string    `json:"micro_lesson_id"`
	Mastery       float64   `json:"mastery"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type NextAction struct {
	SchemaVersion string    `json:"schema_version"`
	ChildID       string    `json:"child_id"`
	LessonID      string    `json:"lesson_id"`
	Action        string    `json:"action"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

func New(cfg Config) *Service {
	if cfg.Root == "" {
		cfg.Root = "."
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Nonce == nil {
		cfg.Nonce = randomNonce
	}
	return &Service{root: cfg.Root, now: cfg.Now, nonce: cfg.Nonce, sessions: map[string]Session{}}
}

func (s *Service) List(_ context.Context, child string, bundles []Bundle) []Bundle {
	var out []Bundle
	for _, b := range bundles {
		if b.ChildID == child && b.Approved && b.Promoted && b.Assigned {
			out = append(out, b)
		}
	}
	return out
}

func (s *Service) Start(_ context.Context, b Bundle, child string) (Session, error) {
	if b.ID == "" || child == "" {
		return Session{}, errors.New("lesson id and child id are required")
	}
	current := ""
	if len(b.MicroLessons) > 0 {
		current = b.MicroLessons[0].ID
	}
	sess := Session{SchemaVersion: SchemaSession, ID: "ls-" + b.ID + "-" + child, Nonce: s.nonce(), ChildID: child, LessonID: b.ID, CurrentMicroID: current}
	s.sessions[sess.ID] = sess
	return sess, nil
}

func (s *Service) Event(_ context.Context, sessionID string, event Event) error {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return errors.New("lesson session not found")
	}
	if event.Nonce != sess.Nonce {
		return errors.New("nonce validation failed")
	}
	if event.ActorRole == "" {
		event.ActorRole = "student"
	}
	if event.MicroLessonID == "" {
		event.MicroLessonID = sess.CurrentMicroID
	}
	rec := TurnRecord{SchemaVersion: SchemaTurn, Timestamp: s.now(), ChildID: nonempty(event.ChildID, sess.ChildID), LessonSession: sess.ID, LessonID: sess.LessonID, MicroLessonID: event.MicroLessonID, AttemptID: event.AttemptID, ActorRole: event.ActorRole, EventType: event.Type, Data: event.Data}
	return appendJSONL(filepath.Join(s.root, "memory", "adaptive", "tutorruntime", "turns.jsonl"), rec)
}

func (s *Service) Advance(_ context.Context, sess Session, b Bundle, verdicts map[string]Verdict) AdvanceResult {
	idx := -1
	for i, ml := range b.MicroLessons {
		if ml.ID == sess.CurrentMicroID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return AdvanceResult{CurrentMicroLessonID: sess.CurrentMicroID, RemediationRef: "tutorruntime/remediation/not-found"}
	}
	ml := b.MicroLessons[idx]
	if !EvidenceSatisfied(ml, verdicts[ml.ID]) {
		return AdvanceResult{CurrentMicroLessonID: ml.ID, RemediationRef: "tutorruntime/remediation/" + ml.ID}
	}
	if idx+1 >= len(b.MicroLessons) {
		return AdvanceResult{Advanced: true, CurrentMicroLessonID: ml.ID}
	}
	next := b.MicroLessons[idx+1].ID
	sess.CurrentMicroID = next
	s.sessions[sess.ID] = sess
	return AdvanceResult{Advanced: true, CurrentMicroLessonID: next}
}

func EvidenceSatisfied(ml MicroLesson, v Verdict) bool {
	if !v.TrustRampPermits {
		return false
	}
	if ml.ObjectiveType == ObjectivePhysicalSkill && len(ml.LearningEvidence.Requires) == 1 && ml.LearningEvidence.Requires[0] == EvidencePhysicalPerformance {
		return v.VisualPass
	}
	for _, req := range ml.LearningEvidence.Requires {
		switch req {
		case EvidencePhysicalPerformance:
			if !v.VisualPass {
				return false
			}
		case EvidenceStudentReasoning:
			if !v.Reasoning {
				return false
			}
		case EvidenceReflection:
			if !v.Reflection {
				return false
			}
		case EvidenceTransfer:
			if !v.Transfer {
				return false
			}
		case EvidenceParentConfirmation:
			if !v.ParentConfirmed {
				return false
			}
		}
	}
	return true
}

func (s *Service) RecordReviewItem(item ReviewItem) error {
	if item.SchemaVersion == "" {
		item.SchemaVersion = SchemaReviewItem
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = s.now()
	}
	return appendJSONL(filepath.Join(s.root, "memory", "lessons", "review_items.jsonl"), item)
}

func (s *Service) RecordProgress(row ProgressRow) error {
	if row.SchemaVersion == "" {
		row.SchemaVersion = SchemaProgress
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = s.now()
	}
	return appendJSONL(filepath.Join(s.root, "memory", "lessons", "progress.jsonl"), row)
}

func (s *Service) RecordNextAction(action NextAction) error {
	if action.SchemaVersion == "" {
		action.SchemaVersion = SchemaNextAction
	}
	if action.CreatedAt.IsZero() {
		action.CreatedAt = s.now()
	}
	return appendJSONL(filepath.Join(s.root, "memory", "lessons", "next_actions.jsonl"), action)
}

func appendJSONL(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(v)
	_, err = f.WriteString(string(data) + "\n")
	return err
}

func randomNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func nonempty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
