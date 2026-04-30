package archive_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
)

func TestArchiveJSONL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ar, _ := archive.New(t.TempDir())
	for i := 0; i < 3; i++ {
		a := adaptive.AdaptiveArtifact{ID: string(rune('a' + i)), Kind: adaptive.ArtifactPrompt, ChildID: "c", CreatedAt: time.Now()}
		if err := ar.AddArtifact(ctx, a); err != nil {
			t.Fatalf("AddArtifact: %v", err)
		}
		if err := ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: a.ID, ChildID: "c", Correctness: true, CombinedScore: float64(i), CreatedAt: time.Now()}); err != nil {
			t.Fatalf("AddOutcome: %v", err)
		}
	}
	reopened, _ := archive.New(ar.Root())
	arts, err := reopened.Artifacts(ctx)
	if err != nil || len(arts) != 3 || arts[0].ID != "a" {
		t.Fatalf("Artifacts = %+v err=%v", arts, err)
	}
	outs, err := reopened.Outcomes(ctx)
	if err != nil || len(outs) != 3 {
		t.Fatalf("Outcomes = %+v err=%v", outs, err)
	}
	f, _ := os.OpenFile(filepath.Join(ar.Root(), "memory", "adaptive", "archive.jsonl"), os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("{bad\n")
	_ = f.Close()
	if _, err := reopened.Artifacts(ctx); err == nil {
		t.Fatal("expected malformed trailing line error")
	}
}

func TestArchiveTopK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ar, _ := archive.New(t.TempDir())
	seed := []adaptive.AdaptiveArtifact{
		{ID: "low", ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "visual"},
		{ID: "high", ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "visual"},
		{ID: "other", ChildID: "other", Subject: "science", Topic: "magnets", Strategy: "visual"},
	}
	for _, a := range seed {
		_ = ar.AddArtifact(ctx, a)
	}
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "low", ChildID: "cross", Correctness: true, CombinedScore: 1, CreatedAt: time.Now()})
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "high", ChildID: "cross", Correctness: true, CombinedScore: 3, CreatedAt: time.Now().Add(time.Second)})
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "other", ChildID: "other", Correctness: true, CombinedScore: 9, CreatedAt: time.Now()})
	top, err := ar.Top(ctx, archive.Query{ChildID: "cross", Subject: "science", Topic: "magnets", Strategy: "visual"}, 2)
	if err != nil || len(top) != 2 || top[0].ID != "high" || top[1].ID != "low" {
		t.Fatalf("Top = %+v err=%v", top, err)
	}
}

func TestArchiveLineageAndFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ar, _ := archive.New(t.TempDir())
	for _, a := range []adaptive.AdaptiveArtifact{{ID: "A"}, {ID: "B", ParentID: "A"}, {ID: "C", ParentID: "B"}, {ID: "bad", Subject: "science", Topic: "magnets"}} {
		_ = ar.AddArtifact(ctx, a)
	}
	_ = ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: "bad", ChildID: "cross", Correctness: false, CombinedScore: -1})
	line, err := ar.Lineage(ctx, "C")
	if err != nil || len(line) != 3 || line[0].ID != "C" || line[2].ID != "A" {
		t.Fatalf("Lineage = %+v err=%v", line, err)
	}
	similar, err := ar.Similar(ctx, adaptive.AdaptiveArtifact{Subject: "science", Topic: "magnets"}, 5)
	if err != nil || len(similar) == 0 || similar[0].ID != "bad" {
		t.Fatalf("Similar = %+v err=%v", similar, err)
	}
	top, err := ar.Top(ctx, archive.Query{ChildID: "cross", Subject: "science", Topic: "magnets"}, 5)
	if err != nil || len(top) != 0 {
		t.Fatalf("Top should exclude failed outcome, got %+v err=%v", top, err)
	}
}
