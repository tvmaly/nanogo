package obs

import (
	"context"
	"fmt"
	"time"
)

type ResearchConfig struct {
	Source string
	Now    func() time.Time
	NewID  func(string) string
}

type ResearchRecorder struct {
	w   Writer
	cfg ResearchConfig
}

type ExperimentRef struct {
	ID   string
	Kind string
}

type Score struct {
	Value  float64
	Rubric string
	Cost   float64
}

type Decision string

const (
	DecisionAccepted Decision = "accepted"
	DecisionRejected Decision = "rejected"
)

type EvalDecision struct {
	CandidateID string
	Decision    Decision
	Reason      string
	Score       Score
}

func NewResearchRecorder(w Writer, cfg ResearchConfig) *ResearchRecorder {
	if cfg.Source == "" {
		cfg.Source = "research"
	}
	return &ResearchRecorder{w: w, cfg: cfg}
}

func (r *ResearchRecorder) ExperimentStarted(ctx context.Context, ref ExperimentRef) error {
	return r.append(ctx, "experiment.start", map[string]any{"experiment_id": ref.ID, "kind": ref.Kind}, nil, nil)
}

func (r *ResearchRecorder) ExperimentFinished(ctx context.Context, ref ExperimentRef, score Score) error {
	return r.append(ctx, "experiment.finish", map[string]any{"experiment_id": ref.ID, "kind": ref.Kind, "score": score}, nil, nil)
}

func (r *ResearchRecorder) ExperimentFailed(ctx context.Context, ref ExperimentRef, err error) error {
	return r.append(ctx, "experiment.failure", map[string]any{"experiment_id": ref.ID, "kind": ref.Kind}, nil, err)
}

func (r *ResearchRecorder) EvalDecision(ctx context.Context, d EvalDecision) error {
	typ := "eval.reject"
	if d.Decision == DecisionAccepted {
		typ = "eval.accept"
	}
	return r.append(ctx, typ, map[string]any{"candidate_id": d.CandidateID, "decision": d.Decision, "reason": d.Reason, "score": d.Score}, nil, nil)
}

func (r *ResearchRecorder) BudgetStopped(ctx context.Context, runID, budget string, limit int) error {
	return r.append(ctx, "budget.stop", map[string]any{"run_id": runID, "budget": budget, "limit": limit}, nil, nil)
}

func (r *ResearchRecorder) PathGuardRejected(ctx context.Context, candidateID, path, reason string) error {
	return r.append(ctx, "path_guard.reject", map[string]any{"candidate_id": candidateID, "path": path, "reason": reason}, nil, nil)
}

func (r *ResearchRecorder) Promoted(ctx context.Context, candidateID, target string) error {
	return r.append(ctx, "promotion", map[string]any{"candidate_id": candidateID, "target": target}, nil, nil)
}

func (r *ResearchRecorder) RolledBack(ctx context.Context, candidateID, target, reason string) error {
	return r.append(ctx, "rollback", map[string]any{"candidate_id": candidateID, "target": target, "reason": reason}, nil, nil)
}

func (r *ResearchRecorder) StudentArtifactCreated(ctx context.Context, studentID string, artifact ArtifactRef) error {
	return r.append(ctx, "student.artifact", map[string]any{"student_id": studentID}, []ArtifactRef{artifact}, nil)
}

func (r *ResearchRecorder) FactExported(ctx context.Context, factID string, artifact ArtifactRef) error {
	return r.append(ctx, "fact.export", map[string]any{"fact_id": factID}, []ArtifactRef{artifact}, nil)
}

func (r *ResearchRecorder) append(ctx context.Context, typ string, attrs map[string]any, artifacts []ArtifactRef, err error) error {
	rec := ObservationRecord{
		SchemaVersion: SchemaVersion,
		ID:            r.id(typ),
		Type:          typ,
		Time:          r.now(),
		Source:        r.cfg.Source,
		Attributes:    attrs,
		Artifacts:     artifacts,
	}
	if err != nil {
		rec.Error = &ErrorInfo{Message: err.Error()}
		rec.Severity = "error"
	}
	return r.w.Append(ctx, rec)
}

func (r *ResearchRecorder) now() time.Time {
	if r.cfg.Now != nil {
		return r.cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func (r *ResearchRecorder) id(typ string) string {
	if r.cfg.NewID != nil {
		return r.cfg.NewID(typ)
	}
	return fmt.Sprintf("%s-%d", typ, r.now().UnixNano())
}
