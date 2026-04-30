package experiment_test

import (
	"testing"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/experiment"
)

func TestIslands(t *testing.T) {
	t.Parallel()
	m := experiment.NewIslandManager(nil)
	a := adaptive.AdaptiveArtifact{ID: "a", Strategy: "visual"}
	assigned := m.Assign(a)
	if assigned.IslandID != "visual" {
		t.Fatalf("IslandID = %q", assigned.IslandID)
	}
	migrated := experiment.Migrate(assigned, "hands_on", "winning visual pattern")
	if migrated.ID != assigned.ID || migrated.IslandID != assigned.IslandID {
		t.Fatalf("migration should not alter original id/island: %+v", migrated)
	}
	if migrated.Metadata["migrated_to"] != "hands_on" {
		t.Fatalf("migration metadata missing: %+v", migrated.Metadata)
	}
}

func TestCandidateSelection(t *testing.T) {
	t.Parallel()
	candidates := []experiment.Candidate{{Artifact: adaptive.AdaptiveArtifact{ID: "a"}, Score: 1}, {Artifact: adaptive.AdaptiveArtifact{ID: "b"}, Score: 5}, {Artifact: adaptive.AdaptiveArtifact{ID: "c"}, Score: 2}}
	s := experiment.NewSelector(experiment.RandFunc(func() float64 { return 0.99 }))
	if got := s.Select(candidates, experiment.SelectOptions{Mode: "best"}).ID; got != "b" {
		t.Fatalf("best = %q", got)
	}
	if got := s.Select(candidates, experiment.SelectOptions{Mode: "round_robin", Key: "kid"}).ID; got != "a" {
		t.Fatalf("round_robin first = %q", got)
	}
	if got := s.Select(candidates, experiment.SelectOptions{Mode: "round_robin", Key: "kid"}).ID; got != "b" {
		t.Fatalf("round_robin second = %q", got)
	}
	if got := s.Select(candidates, experiment.SelectOptions{Mode: "epsilon_greedy", Epsilon: 0}).ID; got != "b" {
		t.Fatalf("epsilon greedy exploit = %q", got)
	}
	explore := experiment.NewSelector(experiment.RandFunc(func() float64 { return 0 }))
	if got := explore.Select(candidates, experiment.SelectOptions{Mode: "epsilon_greedy", Epsilon: 1}).ID; got != "a" {
		t.Fatalf("epsilon greedy explore = %q", got)
	}
	if got := s.Select(candidates, experiment.SelectOptions{Mode: "weighted"}).ID; got == "" {
		t.Fatal("weighted returned empty")
	}
}
