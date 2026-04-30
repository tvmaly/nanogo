// Package tutorruntime adapts live tutoring policy choices from child outcomes.
package tutorruntime

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/eval"
	"github.com/tvmaly/nanogo/ext/adaptive/profile"
	"gopkg.in/yaml.v3"
)

func init() {
	adaptive.RegisterDomain("tutorruntime", func(cfg json.RawMessage) (adaptive.DomainAdapter, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return New(c)
	})
	coretools.Register("tutorruntime", func(cfg json.RawMessage) (coretools.Source, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		rt, err := New(c)
		if err != nil {
			return nil, err
		}
		return Source{runtime: rt}, nil
	})
}

type Config struct {
	Root                                      string           `json:"root"`
	DefaultPolicyID                           string           `json:"default_policy_id"`
	Epsilon                                   float64          `json:"epsilon"`
	Seed                                      int64            `json:"seed"`
	RequireTransferForMastered                bool             `json:"require_transfer_for_mastered"`
	RequireRetentionForMastered               bool             `json:"require_retention_for_mastered"`
	RequireParentApprovalForProfileInference  bool             `json:"require_parent_approval_for_profile_inference"`
	RequireParentApprovalForMajorPolicyChange bool             `json:"required_for_major_policy_change"`
	HintThreshold                             int              `json:"hint_threshold"`
	AttemptThreshold                          int              `json:"attempt_threshold"`
	HomeschoolMode                            bool             `json:"homeschool_mode"`
	SelfLearningMode                          bool             `json:"self_learning_mode"`
	RetentionIntervalsDays                    []int            `json:"retention_intervals_days"`
	Clock                                     func() time.Time `json:"-"`
}

type TutorPolicy struct {
	ID              string         `json:"id" yaml:"id"`
	ChildID         string         `json:"child_id,omitempty" yaml:"child_id,omitempty"`
	Subject         string         `json:"subject,omitempty" yaml:"subject,omitempty"`
	Topic           string         `json:"topic,omitempty" yaml:"topic,omitempty"`
	Strategy        string         `json:"strategy" yaml:"strategy"`
	PromptFile      string         `json:"prompt_file,omitempty" yaml:"prompt_file,omitempty"`
	HintStyle       string         `json:"hint_style,omitempty" yaml:"hint_style,omitempty"`
	QuestionStyle   string         `json:"question_style,omitempty" yaml:"question_style,omitempty"`
	Pacing          string         `json:"pacing,omitempty" yaml:"pacing,omitempty"`
	RemediationMode string         `json:"remediation_mode,omitempty" yaml:"remediation_mode,omitempty"`
	MaxHints        int            `json:"max_hints" yaml:"max_hints"`
	ParentID        string         `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Version         int            `json:"version,omitempty" yaml:"version,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (p TutorPolicy) Validate() error {
	var missing []string
	if p.ID == "" {
		missing = append(missing, "id")
	}
	if p.Strategy == "" {
		missing = append(missing, "strategy")
	}
	if p.MaxHints <= 0 {
		missing = append(missing, "max_hints")
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid tutor policy: missing %s", strings.Join(missing, ", "))
	}
	return nil
}

type Runtime struct {
	root    string
	cfg     Config
	archive *archive.Archive
	profile *profile.Store
	rng     *rand.Rand
	mu      sync.Mutex
}

func New(cfg Config) (*Runtime, error) {
	if cfg.Root == "" {
		cfg.Root = "."
	}
	if cfg.DefaultPolicyID == "" {
		cfg.DefaultPolicyID = "socratic-guide"
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.HintThreshold == 0 {
		cfg.HintThreshold = 3
	}
	if cfg.AttemptThreshold == 0 {
		cfg.AttemptThreshold = 3
	}
	if len(cfg.RetentionIntervalsDays) == 0 {
		cfg.RetentionIntervalsDays = []int{1, 3, 7, 14}
	}
	ar, err := archive.New(cfg.Root)
	if err != nil {
		return nil, err
	}
	ps, err := profile.NewStore(cfg.Root)
	if err != nil {
		return nil, err
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	return &Runtime{root: cfg.Root, cfg: cfg, archive: ar, profile: ps, rng: rand.New(rand.NewSource(cfg.Seed))}, nil
}

func (r *Runtime) Root() string { return r.root }
func (r *Runtime) Name() string { return "tutorruntime" }

func (r *Runtime) ArtifactKinds() []adaptive.ArtifactKind {
	return []adaptive.ArtifactKind{adaptive.ArtifactTutorPolicy, adaptive.ArtifactRubric, adaptive.ArtifactPrompt, adaptive.ArtifactSkill}
}

func (r *Runtime) Compile(ctx context.Context, req adaptive.CompileRequest) ([]adaptive.AdaptiveArtifact, error) {
	policies, err := LoadInitialPolicies()
	if err != nil {
		return nil, err
	}
	var arts []adaptive.AdaptiveArtifact
	for _, p := range policies {
		art := policyArtifact(p, req.ChildID, req.Subject, req.Topic, r.clock())
		arts = append(arts, art)
		_ = r.archive.AddArtifact(ctx, art)
	}
	return arts, nil
}

func (r *Runtime) Evaluate(_ context.Context, a adaptive.AdaptiveArtifact, attempt adaptive.Attempt) (adaptive.AdaptiveEvalResult, error) {
	out := SessionOutcome{
		SessionID: attempt.SessionID, ChildID: attempt.ChildID, PolicyID: a.ID,
		CorrectRate:      num(attempt.Observations["correct_rate"]),
		TransferScore:    num(attempt.Observations["transfer"]),
		RetentionScore:   num(attempt.Observations["retention"]),
		EngagementScore:  num(attempt.Observations["engagement"]),
		FrustrationScore: num(attempt.Observations["frustration"]),
		TimeToMasteryMin: num(attempt.Observations["time_to_mastery_min"]),
		ParentRating:     num(attempt.Observations["parent_rating"]),
	}
	result := r.EvaluateSessionOutcome(out)
	result.ArtifactID = a.ID
	result.AttemptID = attempt.ID
	return result, nil
}

func (r *Runtime) Mutate(ctx context.Context, parent adaptive.AdaptiveArtifact, goal adaptive.MutationGoal) ([]adaptive.AdaptiveArtifact, error) {
	p := TutorPolicy{ID: parent.ID, ParentID: parent.ParentID, Version: parent.Version, ChildID: parent.ChildID, Subject: parent.Subject, Topic: parent.Topic, Strategy: parent.Strategy, MaxHints: 3, Metadata: parent.Metadata}
	child, err := r.MutatePolicy(ctx, p, goal)
	if err != nil {
		return nil, err
	}
	return []adaptive.AdaptiveArtifact{policyArtifact(child, child.ChildID, child.Subject, child.Topic, r.clock())}, nil
}

type PolicyRequest struct {
	ChildID string
	Subject string
	Topic   string
}

type PolicySelection struct {
	Policy          TutorPolicy `json:"policy"`
	Rationale       string      `json:"rationale"`
	PriorOutcomeIDs []string    `json:"prior_outcome_ids,omitempty"`
	Exploration     bool        `json:"exploration,omitempty"`
}

func (r *Runtime) SelectPolicy(ctx context.Context, req PolicyRequest) (PolicySelection, error) {
	if pinned, ok := r.readPin(req.ChildID, req.Subject, req.Topic); ok {
		p := r.policyByID(pinned)
		p.ChildID, p.Subject, p.Topic = req.ChildID, req.Subject, req.Topic
		return PolicySelection{Policy: p, Rationale: "parent-pinned policy"}, nil
	}
	levels := []struct {
		child, subject, topic string
		reason                string
	}{
		{req.ChildID, req.Subject, req.Topic, "best child+subject+topic policy"},
		{req.ChildID, req.Subject, "", "best child+subject policy"},
		{req.ChildID, "", "", "best child-wide policy"},
		{"", "", req.Topic, "best global topic policy"},
	}
	for _, level := range levels {
		top, err := r.topExact(ctx, level.child, level.subject, level.topic)
		if err != nil {
			return PolicySelection{}, err
		}
		if top.ID == "" {
			continue
		}
		p := r.policyByID(top.ID)
		p.ChildID, p.Subject, p.Topic = req.ChildID, req.Subject, req.Topic
		sel := PolicySelection{Policy: p, Rationale: level.reason, PriorOutcomeIDs: []string{top.ID}}
		return r.maybeExplore(ctx, req, sel)
	}
	p := r.policyByID(r.cfg.DefaultPolicyID)
	p.ChildID, p.Subject, p.Topic = req.ChildID, req.Subject, req.Topic
	return PolicySelection{Policy: p, Rationale: "default policy"}, nil
}

func (r *Runtime) topExact(ctx context.Context, child, subject, topic string) (adaptive.AdaptiveArtifact, error) {
	arts, err := r.archive.Artifacts(ctx)
	if err != nil {
		return adaptive.AdaptiveArtifact{}, err
	}
	outs, err := r.archive.Outcomes(ctx)
	if err != nil {
		return adaptive.AdaptiveArtifact{}, err
	}
	latest := map[string]adaptive.AdaptiveEvalResult{}
	for _, out := range outs {
		prev, ok := latest[out.ArtifactID]
		if !ok || out.CreatedAt.After(prev.CreatedAt) {
			latest[out.ArtifactID] = out
		}
	}
	var best adaptive.AdaptiveArtifact
	var bestScore float64
	for _, art := range arts {
		if art.Kind != adaptive.ArtifactTutorPolicy || art.ChildID != child || art.Subject != subject || art.Topic != topic {
			continue
		}
		out, ok := latest[art.ID]
		if !ok || !out.Correctness {
			continue
		}
		if best.ID == "" || out.CombinedScore > bestScore {
			best, bestScore = art, out.CombinedScore
		}
	}
	return best, nil
}

func (r *Runtime) maybeExplore(ctx context.Context, req PolicyRequest, base PolicySelection) (PolicySelection, error) {
	if r.cfg.Epsilon <= 0 {
		return base, nil
	}
	r.mu.Lock()
	roll := r.rng.Float64()
	r.mu.Unlock()
	if roll >= r.cfg.Epsilon {
		return base, nil
	}
	policies, err := LoadInitialPolicies()
	if err != nil {
		return base, err
	}
	var alternatives []TutorPolicy
	for _, p := range policies {
		if p.ID != base.Policy.ID {
			p.ChildID, p.Subject, p.Topic = req.ChildID, req.Subject, req.Topic
			alternatives = append(alternatives, p)
		}
	}
	if len(alternatives) == 0 {
		return base, nil
	}
	r.mu.Lock()
	p := alternatives[r.rng.Intn(len(alternatives))]
	r.mu.Unlock()
	return PolicySelection{Policy: p, Rationale: "epsilon exploration from " + base.Policy.ID, Exploration: true}, nil
}

func (r *Runtime) PinPolicy(_ context.Context, child, subject, topic, policyID string) error {
	path := filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "parent_pins.jsonl")
	return appendJSON(path, map[string]string{"child_id": child, "subject": subject, "topic": topic, "policy_id": policyID})
}

type StartSessionRequest struct {
	ChildID  string
	LessonID string
	Subject  string
	Topic    string
}

type TutorSession struct {
	ID               string    `json:"id"`
	ChildID          string    `json:"child_id"`
	LessonID         string    `json:"lesson_id"`
	LessonArtifactID string    `json:"lesson_artifact_id,omitempty"`
	Subject          string    `json:"subject"`
	Topic            string    `json:"topic"`
	PolicyID         string    `json:"policy_id"`
	PolicyRationale  string    `json:"policy_rationale"`
	StartedAt        time.Time `json:"started_at"`
}

func (r *Runtime) StartSession(ctx context.Context, req StartSessionRequest) (TutorSession, error) {
	if req.LessonID != "" && (req.Subject == "" || req.Topic == "") {
		subject, topic := r.lessonMetadata(req.LessonID)
		if req.Subject == "" {
			req.Subject = subject
		}
		if req.Topic == "" {
			req.Topic = topic
		}
	}
	sel, err := r.SelectPolicy(ctx, PolicyRequest{ChildID: req.ChildID, Subject: req.Subject, Topic: req.Topic})
	if err != nil {
		return TutorSession{}, err
	}
	now := r.clock()
	s := TutorSession{
		ID:      "tutor-" + shortHash(req.ChildID+"|"+req.LessonID+"|"+now.Format(time.RFC3339Nano)),
		ChildID: req.ChildID, LessonID: req.LessonID, LessonArtifactID: req.LessonID,
		Subject: req.Subject, Topic: req.Topic, PolicyID: sel.Policy.ID, PolicyRationale: sel.Rationale, StartedAt: now,
	}
	if err := appendJSON(r.sessionsPath(), s); err != nil {
		return TutorSession{}, err
	}
	return s, nil
}

type TutorTurnOutcome struct {
	SessionID        string    `json:"session_id"`
	ChildID          string    `json:"child_id"`
	PolicyID         string    `json:"policy_id"`
	LessonID         string    `json:"lesson_id"`
	Subject          string    `json:"subject,omitempty"`
	Topic            string    `json:"topic,omitempty"`
	QuestionID       string    `json:"question_id"`
	Correct          bool      `json:"correct"`
	HintCount        int       `json:"hint_count"`
	Attempts         int       `json:"attempts"`
	TimeSeconds      int       `json:"time_seconds"`
	FrustrationScore float64   `json:"frustration_score"`
	EngagementScore  float64   `json:"engagement_score"`
	TransferSuccess  bool      `json:"transfer_success"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (r *Runtime) RecordTurn(_ context.Context, out TutorTurnOutcome) error {
	if out.CreatedAt.IsZero() {
		out.CreatedAt = r.clock()
	}
	return appendJSON(r.turnsPath(), out)
}

func (r *Runtime) Turns(_ context.Context, sessionID string) ([]TutorTurnOutcome, error) {
	all, err := readJSONL[TutorTurnOutcome](r.turnsPath())
	if err != nil {
		return nil, err
	}
	var out []TutorTurnOutcome
	for _, turn := range all {
		if turn.SessionID == sessionID {
			out = append(out, turn)
		}
	}
	return out, nil
}

type GradeRequest struct {
	Answer       string
	Expected     string
	RubricItemID string
}

type GradeResult struct {
	Correct       bool    `json:"correct"`
	PartialCredit float64 `json:"partial_credit"`
	Misconception string  `json:"misconception,omitempty"`
	Feedback      string  `json:"feedback"`
	RubricItemID  string  `json:"rubric_item_id"`
}

func (r *Runtime) GradeAnswer(req GradeRequest) GradeResult {
	answer := strings.ToLower(req.Answer)
	expected := strings.ToLower(req.Expected)
	score := lexicalOverlap(answer, expected)
	result := GradeResult{Correct: score >= 0.34, PartialCredit: score, RubricItemID: req.RubricItemID}
	if strings.Contains(answer, "glue") || strings.Contains(answer, "magic") {
		result.Misconception = "confuses magnetic force with sticky contact"
		result.PartialCredit = min(result.PartialCredit, 0.25)
		result.Correct = false
	}
	if result.Correct {
		result.PartialCredit = 1
		result.Feedback = "Correct: the answer matches the rubric goal."
	} else if result.Misconception != "" {
		result.Feedback = "Review the underlying force, then try a simpler example."
	} else {
		result.Feedback = "Partially correct; add the key idea from the rubric."
	}
	return result
}

type MasteryLevel string

const (
	Unknown        MasteryLevel = "unknown"
	Introduced     MasteryLevel = "introduced"
	Practicing     MasteryLevel = "practicing"
	NearlyMastered MasteryLevel = "nearly_mastered"
	Mastered       MasteryLevel = "mastered"
	NeedsReview    MasteryLevel = "needs_review"
)

type MasteryState struct {
	Level MasteryLevel `json:"level"`
}

type MasteryEvidence struct {
	Correct         bool
	HintCount       int
	TransferSuccess bool
	RetentionCheck  bool
	RetentionPassed bool
}

func (r *Runtime) UpdateMastery(state MasteryState, ev MasteryEvidence) MasteryState {
	if ev.RetentionCheck && !ev.RetentionPassed {
		state.Level = NeedsReview
		return state
	}
	if !ev.Correct || ev.HintCount >= r.cfg.HintThreshold {
		return state
	}
	switch state.Level {
	case "", Unknown:
		state.Level = Introduced
	case Introduced, NeedsReview:
		state.Level = Practicing
	case Practicing:
		if r.cfg.RequireTransferForMastered && !ev.TransferSuccess {
			return state
		}
		state.Level = NearlyMastered
	case NearlyMastered:
		if r.cfg.RequireTransferForMastered && !ev.TransferSuccess {
			return state
		}
		if r.cfg.RequireRetentionForMastered && !ev.RetentionPassed {
			return state
		}
		state.Level = Mastered
	}
	return state
}

type RetentionRequest struct {
	ChildID     string
	Topic       string
	LessonID    string
	QuestionIDs []string
	CompletedAt time.Time
}

type RetentionItem struct {
	ChildID     string    `json:"child_id"`
	Topic       string    `json:"topic"`
	LessonID    string    `json:"lesson_id"`
	DueAt       time.Time `json:"due_at"`
	QuestionIDs []string  `json:"review_question_ids"`
}

func (r *Runtime) ScheduleRetention(_ context.Context, req RetentionRequest) ([]RetentionItem, error) {
	if req.CompletedAt.IsZero() {
		req.CompletedAt = r.clock()
	}
	var items []RetentionItem
	for _, days := range r.cfg.RetentionIntervalsDays {
		item := RetentionItem{ChildID: req.ChildID, Topic: req.Topic, LessonID: req.LessonID, DueAt: req.CompletedAt.AddDate(0, 0, days), QuestionIDs: append([]string(nil), req.QuestionIDs...)}
		items = append(items, item)
		if err := appendJSON(r.retentionPath(), item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

type RemediationInput struct {
	HintCount     int
	Attempts      int
	MissedConcept string
	Prerequisite  string
}

type RemediationRecommendation struct {
	Action        string `json:"action"`
	Reason        string `json:"reason"`
	BlocksMastery bool   `json:"blocks_mastery"`
}

func (r *Runtime) RecommendRemediation(in RemediationInput) RemediationRecommendation {
	if in.Prerequisite != "" {
		return RemediationRecommendation{Action: "review_prerequisite", Reason: "prerequisite gap: " + in.Prerequisite, BlocksMastery: true}
	}
	if in.Attempts >= r.cfg.AttemptThreshold {
		return RemediationRecommendation{Action: "switch_to_visual", Reason: "attempt threshold reached", BlocksMastery: true}
	}
	if in.HintCount >= r.cfg.HintThreshold {
		return RemediationRecommendation{Action: "show_worked_example", Reason: "hint threshold reached", BlocksMastery: true}
	}
	return RemediationRecommendation{Action: "give_smaller_hint", Reason: "continue with lighter support"}
}

type StrategySwitch struct {
	SessionID   string `json:"session_id"`
	OldPolicyID string `json:"old_policy_id"`
	NewPolicyID string `json:"new_policy_id"`
	Reason      string `json:"reason"`
}

func (r *Runtime) MaybeSwitchStrategy(_ context.Context, sessionID string, turn TutorTurnOutcome) (StrategySwitch, error) {
	if turn.Attempts < r.cfg.AttemptThreshold {
		return StrategySwitch{SessionID: sessionID, OldPolicyID: turn.PolicyID}, nil
	}
	newID := "worked-example-first"
	if turn.PolicyID == newID {
		newID = "visual-analogy"
	}
	sw := StrategySwitch{SessionID: sessionID, OldPolicyID: turn.PolicyID, NewPolicyID: newID, Reason: "attempt threshold reached"}
	return sw, appendJSON(r.switchesPath(), sw)
}

func (r *Runtime) RecordMisconception(ctx context.Context, child, topic, text string) error {
	ch, err := r.profile.Propose(ctx, profile.Change{ChildID: child, Field: "misconception:" + topic, Proposed: text})
	if err != nil {
		return err
	}
	if !r.cfg.RequireParentApprovalForProfileInference {
		return r.profile.Resolve(ctx, ch.ID, profile.Approved, "")
	}
	return nil
}

func (r *Runtime) PendingProfileChanges(ctx context.Context, child string) ([]profile.Change, error) {
	return r.profile.Changes(ctx, child)
}

type SessionOutcome struct {
	SessionID        string
	ChildID          string
	PolicyID         string
	CorrectRate      float64
	TransferScore    float64
	RetentionScore   float64
	EngagementScore  float64
	FrustrationScore float64
	TimeToMasteryMin float64
	ParentRating     float64
}

func (r *Runtime) EvaluateSessionOutcome(out SessionOutcome) adaptive.AdaptiveEvalResult {
	result := adaptive.AdaptiveEvalResult{
		ArtifactID: out.PolicyID, AttemptID: out.SessionID, ChildID: out.ChildID,
		Correctness: out.CorrectRate >= 0.7, QualityScore: out.CorrectRate, MasteryGain: out.CorrectRate,
		RetentionScore: out.RetentionScore, TransferScore: out.TransferScore, EngagementScore: out.EngagementScore,
		FrustrationScore: out.FrustrationScore, TimeToMasteryMin: out.TimeToMasteryMin, ParentRating: out.ParentRating / 5,
		Notes: "tutorruntime session outcome", CreatedAt: r.clock(),
	}
	return eval.Score(result, eval.DefaultScoreConfig())
}

func (r *Runtime) MutatePolicy(ctx context.Context, parent TutorPolicy, goal adaptive.MutationGoal) (TutorPolicy, error) {
	if parent.ID == "" {
		return TutorPolicy{}, errors.New("parent policy id required")
	}
	child := parent
	child.ParentID = parent.ID
	child.Version = max(parent.Version+1, 2)
	child.ID = parent.ID + "-v" + fmt.Sprint(child.Version)
	if child.Metadata == nil {
		child.Metadata = map[string]any{}
	}
	child.Metadata["mutation_goal"] = strings.Join(goal.Improve, ", ")
	child.Metadata["rationale"] = "adaptive tutor runtime mutation"
	path := filepath.Join(r.root, "memory", "adaptive", "artifacts", "tutor_policies", child.ID+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return TutorPolicy{}, err
	}
	data, err := yaml.Marshal(child)
	if err != nil {
		return TutorPolicy{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return TutorPolicy{}, err
	}
	_ = r.archive.AddArtifact(ctx, policyArtifact(child, child.ChildID, child.Subject, child.Topic, r.clock()))
	return child, nil
}

type PolicyActivation struct {
	ChildID                string `json:"child_id"`
	OldPolicyID            string `json:"old_policy_id"`
	NewPolicyID            string `json:"new_policy_id"`
	Active                 bool   `json:"active"`
	RequiresParentApproval bool   `json:"requires_parent_approval"`
}

func (r *Runtime) ProposePolicyChange(_ context.Context, child, oldPolicy, newPolicy string) (PolicyActivation, error) {
	major := family(oldPolicy) != family(newPolicy)
	act := PolicyActivation{ChildID: child, OldPolicyID: oldPolicy, NewPolicyID: newPolicy, Active: true}
	if major && r.cfg.RequireParentApprovalForMajorPolicyChange {
		act.Active = false
		act.RequiresParentApproval = true
	}
	return act, appendJSON(r.activationsPath(), act)
}

type SessionSummary struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Markdown  string `json:"markdown"`
}

func (r *Runtime) SummarizeSession(ctx context.Context, sessionID string) (SessionSummary, error) {
	s, err := r.session(sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	turns, err := r.Turns(ctx, sessionID)
	if err != nil {
		return SessionSummary{}, err
	}
	hints, correct := 0, 0
	var notes []string
	for _, turn := range turns {
		hints += turn.HintCount
		if turn.Correct {
			correct++
		}
		if turn.Notes != "" {
			notes = append(notes, turn.Notes)
		}
	}
	md := fmt.Sprintf("# Tutoring Session Summary\n\nChild %s worked on %s / %s.\n\nWhat the child understood: %d of %d responses were correct. %s\n\nWhat the child struggled with: review missed or high-hint questions.\n\nHints used: %d\n\nMisconceptions: see child profile memory when recorded.\n\nMastery movement: evidence recorded for %s.\n\nRecommended next step: schedule retention review and continue with %s.\n\nParent action needed: review any pending profile or major policy changes.\n", s.ChildID, s.Subject, s.Topic, correct, len(turns), strings.Join(notes, "; "), hints, s.Topic, s.PolicyID)
	path := filepath.Join(r.root, "memory", "adaptive", "reports", "tutorruntime", sessionID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return SessionSummary{}, err
	}
	if err := os.WriteFile(path, []byte(md), 0644); err != nil {
		return SessionSummary{}, err
	}
	return SessionSummary{SessionID: sessionID, Path: path, Markdown: md}, nil
}

type HomeschoolEvidence struct {
	Date               string `json:"date"`
	ChildID            string `json:"child_id"`
	Subject            string `json:"subject"`
	Topic              string `json:"topic"`
	LessonID           string `json:"lesson_id"`
	QuestionsAttempted int    `json:"questions_attempted"`
	MasteryMovement    string `json:"mastery_movement"`
	SummaryPath        string `json:"summary_path"`
}

func (r *Runtime) HomeschoolEvidence(ctx context.Context, sessionID string) (HomeschoolEvidence, error) {
	s, err := r.session(sessionID)
	if err != nil {
		return HomeschoolEvidence{}, err
	}
	turns, err := r.Turns(ctx, sessionID)
	if err != nil {
		return HomeschoolEvidence{}, err
	}
	summary, err := r.SummarizeSession(ctx, sessionID)
	if err != nil {
		return HomeschoolEvidence{}, err
	}
	ev := HomeschoolEvidence{Date: s.StartedAt.Format("2006-01-02"), ChildID: s.ChildID, Subject: s.Subject, Topic: s.Topic, LessonID: s.LessonID, QuestionsAttempted: len(turns), MasteryMovement: "evidence recorded", SummaryPath: summary.Path}
	return ev, appendJSON(filepath.Join(r.root, "memory", "adaptive", "reports", "tutorruntime", "homeschool_evidence.jsonl"), ev)
}

func (r *Runtime) ArchiveSessionOutcome(ctx context.Context, sessionID string) error {
	s, err := r.session(sessionID)
	if err != nil {
		return err
	}
	turns, err := r.Turns(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(turns) == 0 {
		return errors.New("no turns recorded")
	}
	correct, engagement, frustration, transfer := 0, 0.0, 0.0, 0.0
	for _, turn := range turns {
		if turn.Correct {
			correct++
		}
		engagement += turn.EngagementScore
		frustration += turn.FrustrationScore
		if turn.TransferSuccess {
			transfer = 1
		}
	}
	out := r.EvaluateSessionOutcome(SessionOutcome{SessionID: s.ID, ChildID: s.ChildID, PolicyID: s.PolicyID, CorrectRate: float64(correct) / float64(len(turns)), TransferScore: transfer, RetentionScore: 0.5, EngagementScore: engagement / float64(len(turns)), FrustrationScore: frustration / float64(len(turns)), TimeToMasteryMin: 10})
	if err := r.archive.AddOutcome(ctx, out); err != nil {
		return err
	}
	lessonOut := out
	lessonOut.ArtifactID = s.LessonArtifactID
	lessonOut.Notes = "tutorruntime lesson outcome"
	return r.archive.AddOutcome(ctx, lessonOut)
}

type Source struct{ runtime *Runtime }

func (s Source) Tools(context.Context, coretools.TurnInfo) ([]coretools.Tool, error) {
	names := []string{"tutorruntime_select_policy", "tutorruntime_record_turn", "tutorruntime_grade_answer", "tutorruntime_update_mastery", "tutorruntime_detect_misconception", "tutorruntime_recommend_remediation", "tutorruntime_schedule_review", "tutorruntime_summarize_session"}
	out := make([]coretools.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, runtimeTool{name: n, source: s})
	}
	return out, nil
}

type runtimeTool struct {
	name   string
	source Source
}

func (t runtimeTool) Name() string { return t.name }
func (t runtimeTool) Schema() json.RawMessage {
	data, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]any{"name": t.name, "description": "Adaptive tutor runtime operation", "parameters": map[string]any{"type": "object"}}})
	return data
}
func (t runtimeTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	switch t.name {
	case "tutorruntime_select_policy":
		var in PolicyRequest
		_ = json.Unmarshal(args, &in)
		out, err := t.source.runtime.SelectPolicy(ctx, in)
		return encode(out), err
	case "tutorruntime_record_turn":
		var in TutorTurnOutcome
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		return `{"ok":true}`, t.source.runtime.RecordTurn(ctx, in)
	case "tutorruntime_grade_answer":
		var in GradeRequest
		_ = json.Unmarshal(args, &in)
		return encode(t.source.runtime.GradeAnswer(in)), nil
	case "tutorruntime_update_mastery":
		var in struct {
			State    MasteryState
			Evidence MasteryEvidence
		}
		_ = json.Unmarshal(args, &in)
		return encode(t.source.runtime.UpdateMastery(in.State, in.Evidence)), nil
	case "tutorruntime_detect_misconception":
		var in struct{ ChildID, Topic, Text string }
		_ = json.Unmarshal(args, &in)
		return `{"ok":true}`, t.source.runtime.RecordMisconception(ctx, in.ChildID, in.Topic, in.Text)
	case "tutorruntime_recommend_remediation":
		var in RemediationInput
		_ = json.Unmarshal(args, &in)
		return encode(t.source.runtime.RecommendRemediation(in)), nil
	case "tutorruntime_schedule_review":
		var in RetentionRequest
		_ = json.Unmarshal(args, &in)
		out, err := t.source.runtime.ScheduleRetention(ctx, in)
		return encode(out), err
	case "tutorruntime_summarize_session":
		var in struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(args, &in)
		out, err := t.source.runtime.SummarizeSession(ctx, in.SessionID)
		return encode(out), err
	default:
		return "", fmt.Errorf("unknown tutorruntime tool %q", t.name)
	}
}

func LoadInitialPolicies() ([]TutorPolicy, error) {
	var out []TutorPolicy
	for _, body := range initialPolicyYAML {
		var p TutorPolicy
		if err := yaml.Unmarshal([]byte(body), &p); err != nil {
			return nil, err
		}
		if p.Version == 0 {
			p.Version = 1
		}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

var initialPolicyYAML = []string{
	"id: socratic-guide\nstrategy: socratic\nprompt_file: policies/socratic-guide.md\nhint_style: questions_first\nquestion_style: conceptual\npacing: steady\nremediation_mode: smaller_questions\nmax_hints: 3\n",
	"id: worked-example-first\nstrategy: worked_example\nprompt_file: policies/worked-example-first.md\nhint_style: model_then_try\nquestion_style: guided_practice\npacing: deliberate\nremediation_mode: show_worked_example\nmax_hints: 4\n",
	"id: hands-on-remediation\nstrategy: hands_on\nprompt_file: policies/hands-on-remediation.md\nhint_style: concrete_materials\nquestion_style: observation\npacing: slow\nremediation_mode: switch_to_hands_on\nmax_hints: 4\n",
	"id: visual-analogy\nstrategy: visual_analogy\nprompt_file: policies/visual-analogy.md\nhint_style: diagrams\nquestion_style: compare_contrast\npacing: steady\nremediation_mode: switch_to_visual\nmax_hints: 3\n",
	"id: story-explanation\nstrategy: story\nprompt_file: policies/story-explanation.md\nhint_style: narrative\nquestion_style: retell\npacing: warm\nremediation_mode: story_reframe\nmax_hints: 3\n",
	"id: retrieval-practice\nstrategy: retrieval_practice\nprompt_file: policies/retrieval-practice.md\nhint_style: minimal\nquestion_style: recall_then_transfer\npacing: brisk\nremediation_mode: review_prerequisite\nmax_hints: 2\n",
	"id: challenge-mode\nstrategy: challenge\nprompt_file: policies/challenge-mode.md\nhint_style: delayed\nquestion_style: extension\npacing: brisk\nremediation_mode: ask_simpler_question\nmax_hints: 2\n",
	"id: gentle-coach\nstrategy: gentle_coach\nprompt_file: policies/gentle-coach.md\nhint_style: encouraging\nquestion_style: confidence_building\npacing: slow\nremediation_mode: pause_and_encourage\nmax_hints: 5\n",
}

func (r *Runtime) policyByID(id string) TutorPolicy {
	policies, _ := LoadInitialPolicies()
	for _, p := range policies {
		if p.ID == id {
			return p
		}
	}
	return TutorPolicy{ID: id, Strategy: strings.ReplaceAll(id, "-", "_"), MaxHints: 3, Version: 1}
}

func policyArtifact(p TutorPolicy, child, subject, topic string, now time.Time) adaptive.AdaptiveArtifact {
	return adaptive.AdaptiveArtifact{ID: p.ID, Kind: adaptive.ArtifactTutorPolicy, Version: max(p.Version, 1), ChildID: nonempty(p.ChildID, child), Subject: nonempty(p.Subject, subject), Topic: nonempty(p.Topic, topic), Strategy: p.Strategy, ParentID: p.ParentID, Files: []string{p.PromptFile}, Metadata: p.Metadata, CreatedAt: now}
}

func (r *Runtime) readPin(child, subject, topic string) (string, bool) {
	var pins []map[string]string
	_ = readJSONLInto(filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "parent_pins.jsonl"), &pins)
	for i := len(pins) - 1; i >= 0; i-- {
		p := pins[i]
		if p["child_id"] == child && p["subject"] == subject && p["topic"] == topic {
			return p["policy_id"], true
		}
	}
	return "", false
}

func (r *Runtime) lessonMetadata(id string) (string, string) {
	data, err := os.ReadFile(filepath.Join(r.root, "lessons", "generated", id, "lesson.yaml"))
	if err != nil {
		return "", ""
	}
	var subject, topic string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "subject: ") {
			subject = strings.TrimPrefix(line, "subject: ")
		}
		if strings.HasPrefix(line, "topic: ") {
			topic = strings.TrimPrefix(line, "topic: ")
		}
	}
	return subject, topic
}

func (r *Runtime) session(id string) (TutorSession, error) {
	sessions, err := readJSONL[TutorSession](r.sessionsPath())
	if err != nil {
		return TutorSession{}, err
	}
	for _, s := range sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return TutorSession{}, fmt.Errorf("session %q not found", id)
}

func (r *Runtime) clock() time.Time { return r.cfg.Clock().UTC() }
func (r *Runtime) sessionsPath() string {
	return filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "sessions.jsonl")
}
func (r *Runtime) turnsPath() string {
	return filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "turns.jsonl")
}
func (r *Runtime) retentionPath() string {
	return filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "pending_reviews.jsonl")
}
func (r *Runtime) switchesPath() string {
	return filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "strategy_switches.jsonl")
}
func (r *Runtime) activationsPath() string {
	return filepath.Join(r.root, "memory", "adaptive", "tutorruntime", "policy_activations.jsonl")
}

func appendJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if sc.Text() == "" {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func readJSONLInto(path string, out any) error {
	data, err := readJSONL[json.RawMessage](path)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(data)
	return json.Unmarshal(b, out)
}

func encode(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func lexicalOverlap(a, b string) float64 {
	aw, bw := words(a), words(b)
	if len(bw) == 0 {
		return 0
	}
	hits := 0
	for w := range bw {
		if aw[w] {
			hits++
		}
	}
	return float64(hits) / float64(len(bw))
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(part) > 2 {
			out[part] = true
		}
	}
	return out
}

func family(id string) string {
	switch id {
	case "challenge-mode":
		return "challenge"
	case "gentle-coach":
		return "supportive"
	default:
		return strings.Split(id, "-")[0]
	}
}

func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
}

func nonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
