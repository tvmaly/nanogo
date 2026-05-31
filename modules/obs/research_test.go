package obs_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/modules/obs"
)

func TestResearchHelpersRecordAcceptanceScenarios(t *testing.T) {
	ctx := context.Background()
	store := obs.NewFakeStore()
	recorder := obs.NewResearchRecorder(store, obs.ResearchConfig{Source: "acceptance-test"})

	calls := []func(context.Context) error{
		func(ctx context.Context) error {
			return recorder.ExperimentStarted(ctx, obs.ExperimentRef{ID: "exp-1", Kind: "prompt"})
		},
		func(ctx context.Context) error {
			return recorder.ExperimentFinished(ctx, obs.ExperimentRef{ID: "exp-1", Kind: "prompt"}, obs.Score{Value: 0.8, Rubric: "quality"})
		},
		func(ctx context.Context) error {
			return recorder.ExperimentFailed(ctx, obs.ExperimentRef{ID: "exp-2", Kind: "tool"}, errorsForTest("failed eval"))
		},
		func(ctx context.Context) error {
			return recorder.EvalDecision(ctx, obs.EvalDecision{CandidateID: "cand-1", Decision: obs.DecisionAccepted, Score: obs.Score{Value: 0.9}})
		},
		func(ctx context.Context) error {
			return recorder.EvalDecision(ctx, obs.EvalDecision{CandidateID: "cand-2", Decision: obs.DecisionRejected, Reason: "below baseline"})
		},
		func(ctx context.Context) error {
			return recorder.BudgetStopped(ctx, "run-1", "tokens", 100)
		},
		func(ctx context.Context) error {
			return recorder.PathGuardRejected(ctx, "cand-3", "core/agent/loop.go", "core rejected")
		},
		func(ctx context.Context) error {
			return recorder.Promoted(ctx, "cand-1", "active-policy")
		},
		func(ctx context.Context) error {
			return recorder.RolledBack(ctx, "cand-1", "baseline-policy", "regression")
		},
		func(ctx context.Context) error {
			return recorder.StudentArtifactCreated(ctx, "student-1", obs.ArtifactRef{Kind: "lesson", URI: "workspace/lessons/a.md"})
		},
		func(ctx context.Context) error {
			return recorder.FactExported(ctx, "fact-1", obs.ArtifactRef{Kind: "jsonl", URI: "workspace/context/facts.jsonl"})
		},
	}
	for _, call := range calls {
		if err := call(ctx); err != nil {
			t.Fatalf("record helper: %v", err)
		}
	}

	got, err := store.Query(ctx, obs.QuerySpec{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	wantTypes := []string{
		"experiment.start", "experiment.finish", "experiment.failure", "eval.accept", "eval.reject",
		"budget.stop", "path_guard.reject", "promotion", "rollback", "student.artifact", "fact.export",
	}
	if len(got.Records) != len(wantTypes) {
		t.Fatalf("record count = %d, want %d", len(got.Records), len(wantTypes))
	}
	for i, typ := range wantTypes {
		if got.Records[i].Type != typ {
			t.Fatalf("record[%d].Type = %q, want %q", i, got.Records[i].Type, typ)
		}
	}
}

type errorsForTest string

func (e errorsForTest) Error() string { return string(e) }
