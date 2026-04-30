package artifact_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive/artifact"
)

func TestArtifactRoundtrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	values := []any{
		artifact.AdaptiveArtifact{ID: "a1", Kind: artifact.ArtifactPrompt, Version: 2, ChildID: "c1", Subject: "math", Topic: "fractions", Strategy: "visual", Files: []string{"prompt.md"}, Metadata: map[string]any{"unknown": "kept"}, CreatedAt: now},
		artifact.Attempt{ID: "try1", ArtifactID: "a1", ChildID: "c1", SessionID: "s1", StartedAt: now, CompletedAt: now.Add(time.Minute), Inputs: map[string]any{"x": "y"}, Observations: map[string]any{"score": 0.7}},
		artifact.AdaptiveEvalResult{ArtifactID: "a1", AttemptID: "try1", ChildID: "c1", Correctness: true, MasteryGain: 0.3, CombinedScore: 1.2, Metrics: map[string]float64{"retention": 0.4}, CreatedAt: now},
		artifact.MutationGoal{ChildID: "c1", Subject: "math", Topic: "fractions", Improve: []string{"retention"}, Avoid: []string{"frustration"}, Constraints: []string{"short"}},
		artifact.ExperimentPlan{ID: "e1", Domain: "fake", ChildID: "c1", Subject: "math", Topic: "fractions", CandidateIDs: []string{"a1"}, SelectionMode: "best", SuccessMetrics: []string{"mastery"}, CreatedAt: now},
	}
	for _, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", v, err)
		}
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("Unmarshal(%T): %v", v, err)
		}
	}
}
