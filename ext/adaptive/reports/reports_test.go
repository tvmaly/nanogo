package reports_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/reports"
)

func TestChildPatternSummary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	ar, _ := archive.New(root)
	_ = ar.AddArtifact(ctx, adaptive.AdaptiveArtifact{ID: "win", ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "hands_on"})
	_ = ar.AddArtifact(ctx, adaptive.AdaptiveArtifact{ID: "fail", ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "socratic"})
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "win", ChildID: "cross", Correctness: true, CombinedScore: 5, Notes: "built a compass", CreatedAt: time.Now()})
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "fail", ChildID: "cross", Correctness: false, CombinedScore: -1, Notes: "too abstract", CreatedAt: time.Now()})
	path, err := reports.WriteChildPatternSummary(ctx, root, ar, "cross")
	if err != nil {
		t.Fatalf("WriteChildPatternSummary: %v", err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{"# Child Patterns: cross", "Winning patterns", "Failed patterns", "Subject-specific notes", "Current hypotheses", "hands_on", "socratic"} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q:\n%s", want, text)
		}
	}
}

func TestAdaptiveInspect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	ar, _ := archive.New(root)
	_ = ar.AddArtifact(ctx, adaptive.AdaptiveArtifact{ID: "a", ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "visual"})
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "a", ChildID: "cross", Correctness: true, CombinedScore: 2, MasteryGain: .4, EngagementScore: .8, CreatedAt: time.Now()})
	path, err := reports.Inspect(ctx, root, ar, reports.InspectQuery{ChildID: "cross", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	lower := strings.ToLower(text)
	for _, want := range []string{"top artifacts", "score", "lineage", "why it won", "parent approval", "recommended next experiment"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("inspect missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "meta_scratchpad") {
		t.Fatal("inspect exposed private scratchpad")
	}
}
