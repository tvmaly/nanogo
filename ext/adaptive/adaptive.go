// Package adaptive defines the shared extension types for adaptive experiments.
package adaptive

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ArtifactKind string

const (
	ArtifactLessonBundle ArtifactKind = "lesson_bundle"
	ArtifactPathway      ArtifactKind = "lesson_pathway"
	ArtifactTutorPolicy  ArtifactKind = "tutor_policy"
	ArtifactRubric       ArtifactKind = "rubric"
	ArtifactPrompt       ArtifactKind = "prompt"
	ArtifactSkill        ArtifactKind = "skill"
	ArtifactTemplate     ArtifactKind = "template"
)

type AdaptiveArtifact struct {
	ID        string         `json:"id"`
	Kind      ArtifactKind   `json:"kind"`
	Version   int            `json:"version"`
	ChildID   string         `json:"child_id,omitempty"`
	Subject   string         `json:"subject,omitempty"`
	Topic     string         `json:"topic,omitempty"`
	AgeBand   string         `json:"age_band,omitempty"`
	Strategy  string         `json:"strategy,omitempty"`
	Files     []string       `json:"files,omitempty"`
	ParentID  string         `json:"parent_id,omitempty"`
	IslandID  string         `json:"island_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Attempt struct {
	ID           string         `json:"id"`
	ArtifactID   string         `json:"artifact_id"`
	ChildID      string         `json:"child_id"`
	SessionID    string         `json:"session_id,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	CompletedAt  time.Time      `json:"completed_at,omitempty"`
	Inputs       map[string]any `json:"inputs,omitempty"`
	Observations map[string]any `json:"observations,omitempty"`
}

type AdaptiveEvalResult struct {
	ArtifactID       string             `json:"artifact_id"`
	AttemptID        string             `json:"attempt_id,omitempty"`
	ChildID          string             `json:"child_id"`
	Correctness      bool               `json:"correctness"`
	QualityScore     float64            `json:"quality_score"`
	MasteryGain      float64            `json:"mastery_gain"`
	RetentionScore   float64            `json:"retention_score"`
	TransferScore    float64            `json:"transfer_score"`
	EngagementScore  float64            `json:"engagement_score"`
	FrustrationScore float64            `json:"frustration_score"`
	TimeToMasteryMin float64            `json:"time_to_mastery_min"`
	ParentRating     float64            `json:"parent_rating"`
	CostUSD          float64            `json:"cost_usd"`
	CombinedScore    float64            `json:"combined_score"`
	Notes            string             `json:"notes,omitempty"`
	Metrics          map[string]float64 `json:"metrics,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
}

type MutationGoal struct {
	ChildID     string   `json:"child_id,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	Topic       string   `json:"topic,omitempty"`
	Improve     []string `json:"improve,omitempty"`
	Avoid       []string `json:"avoid,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

type ExperimentPlan struct {
	ID             string    `json:"id"`
	Domain         string    `json:"domain"`
	ChildID        string    `json:"child_id"`
	Subject        string    `json:"subject"`
	Topic          string    `json:"topic"`
	CandidateIDs   []string  `json:"candidate_ids,omitempty"`
	SelectionMode  string    `json:"selection_mode"`
	SuccessMetrics []string  `json:"success_metrics,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type CompileRequest struct {
	Domain     string         `json:"domain,omitempty"`
	ChildID    string         `json:"child_id"`
	Subject    string         `json:"subject"`
	Topic      string         `json:"topic"`
	SourcePath string         `json:"source_path,omitempty"`
	SourceBody string         `json:"source_body,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
}

type DomainAdapter interface {
	Name() string
	ArtifactKinds() []ArtifactKind
	Compile(context.Context, CompileRequest) ([]AdaptiveArtifact, error)
	Evaluate(context.Context, AdaptiveArtifact, Attempt) (AdaptiveEvalResult, error)
	Mutate(context.Context, AdaptiveArtifact, MutationGoal) ([]AdaptiveArtifact, error)
}

type AdapterFactory func(json.RawMessage) (DomainAdapter, error)

var (
	regMu   sync.RWMutex
	domains = map[string]AdapterFactory{}
)

func RegisterDomain(name string, f AdapterFactory) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := domains[name]; ok {
		panic("adaptive domain already registered: " + name)
	}
	domains[name] = f
}

func BuildDomain(name string, cfg json.RawMessage) (DomainAdapter, error) {
	regMu.RLock()
	f, ok := domains[name]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown adaptive domain: %q", name)
	}
	return f(cfg)
}

type FakeDomain struct{}

func (FakeDomain) Name() string { return "fake" }

func (FakeDomain) ArtifactKinds() []ArtifactKind {
	return []ArtifactKind{ArtifactPrompt, ArtifactTemplate}
}

func (FakeDomain) Compile(_ context.Context, req CompileRequest) ([]AdaptiveArtifact, error) {
	now := time.Now().UTC()
	return []AdaptiveArtifact{
		{ID: "fake-" + req.ChildID + "-visual", Kind: ArtifactPrompt, Version: 1, ChildID: req.ChildID, Subject: req.Subject, Topic: req.Topic, Strategy: "visual", IslandID: "visual", Metadata: map[string]any{"source": req.SourceBody}, CreatedAt: now},
		{ID: "fake-" + req.ChildID + "-hands-on", Kind: ArtifactPrompt, Version: 1, ChildID: req.ChildID, Subject: req.Subject, Topic: req.Topic, Strategy: "hands_on", IslandID: "hands_on", Metadata: map[string]any{"source": req.SourceBody}, CreatedAt: now},
	}, nil
}

func (FakeDomain) Evaluate(_ context.Context, a AdaptiveArtifact, attempt Attempt) (AdaptiveEvalResult, error) {
	score, _ := attempt.Observations["score"].(float64)
	if score == 0 {
		score = 0.5
	}
	return AdaptiveEvalResult{
		ArtifactID: a.ID, AttemptID: attempt.ID, ChildID: attempt.ChildID,
		Correctness: score >= 0.5, MasteryGain: score, RetentionScore: score / 2,
		TransferScore: score / 2, EngagementScore: score, QualityScore: score,
		CombinedScore: score*4 + score, Notes: "fake domain evaluation",
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (FakeDomain) Mutate(_ context.Context, parent AdaptiveArtifact, goal MutationGoal) ([]AdaptiveArtifact, error) {
	child := parent
	child.ID = parent.ID + "-v" + fmt.Sprint(parent.Version+1)
	child.ParentID = parent.ID
	child.Version = parent.Version + 1
	child.CreatedAt = time.Now().UTC()
	if child.Metadata == nil {
		child.Metadata = map[string]any{}
	}
	child.Metadata["mutation_goal"] = goal.Improve
	return []AdaptiveArtifact{child}, nil
}
