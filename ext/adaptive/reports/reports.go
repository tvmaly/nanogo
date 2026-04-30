// Package reports writes parent-readable adaptive summaries.
package reports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
)

type InspectQuery struct {
	ChildID string
	Subject string
	Topic   string
}

func WriteChildPatternSummary(ctx context.Context, root string, ar *archive.Archive, childID string) (string, error) {
	arts, err := ar.Artifacts(ctx)
	if err != nil {
		return "", err
	}
	outs, err := ar.Outcomes(ctx)
	if err != nil {
		return "", err
	}
	byArt := map[string]adaptive.AdaptiveArtifact{}
	for _, art := range arts {
		byArt[art.ID] = art
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].CombinedScore > outs[j].CombinedScore })
	var b strings.Builder
	fmt.Fprintf(&b, "# Child Patterns: %s\n\n", childID)
	b.WriteString("## Winning patterns\n\n")
	for _, out := range outs {
		if out.ChildID == childID && out.Correctness {
			art := byArt[out.ArtifactID]
			fmt.Fprintf(&b, "- %s/%s via %s scored %.2f. %s\n", art.Subject, art.Topic, art.Strategy, out.CombinedScore, out.Notes)
		}
	}
	b.WriteString("\n## Failed patterns\n\n")
	for _, out := range outs {
		if out.ChildID == childID && !out.Correctness {
			art := byArt[out.ArtifactID]
			fmt.Fprintf(&b, "- %s/%s via %s scored %.2f. %s\n", art.Subject, art.Topic, art.Strategy, out.CombinedScore, out.Notes)
		}
	}
	b.WriteString("\n## Subject-specific notes\n\n")
	subjects := map[string]bool{}
	for _, art := range arts {
		if art.ChildID == childID {
			subjects[art.Subject] = true
		}
	}
	for subj := range subjects {
		fmt.Fprintf(&b, "- %s: prefer strategies with repeated positive outcomes.\n", subj)
	}
	b.WriteString("\n## Current hypotheses\n\n- Continue comparing top strategies against at least one alternative before making long-term profile changes.\n")
	path := filepath.Join(root, "memory", "adaptive", "child_patterns", childID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0644)
}

func Inspect(ctx context.Context, root string, ar *archive.Archive, q InspectQuery) (string, error) {
	top, err := ar.Top(ctx, archive.Query{ChildID: q.ChildID, Subject: q.Subject, Topic: q.Topic}, 5)
	if err != nil {
		return "", err
	}
	outs, err := ar.Outcomes(ctx)
	if err != nil {
		return "", err
	}
	latest := map[string]adaptive.AdaptiveEvalResult{}
	for _, out := range outs {
		if prev, ok := latest[out.ArtifactID]; !ok || out.CreatedAt.After(prev.CreatedAt) {
			latest[out.ArtifactID] = out
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Adaptive Inspect: %s %s %s\n\n", q.ChildID, q.Subject, q.Topic)
	b.WriteString("## Top artifacts\n\n")
	for _, art := range top {
		out := latest[art.ID]
		lineage, _ := ar.Lineage(ctx, art.ID)
		fmt.Fprintf(&b, "- `%s` strategy=%s score=%.2f metrics mastery=%.2f engagement=%.2f\n", art.ID, art.Strategy, out.CombinedScore, out.MasteryGain, out.EngagementScore)
		fmt.Fprintf(&b, "  - lineage: %s\n", lineageIDs(lineage))
		fmt.Fprintf(&b, "  - why it won or lost: score %.2f with correctness=%t\n", out.CombinedScore, out.Correctness)
		fmt.Fprintf(&b, "  - parent approval status: not requested\n")
	}
	b.WriteString("\n## Recommended next experiment\n\nCompare the current winner against a contrasting strategy with the same topic and a short retention check.\n")
	path := filepath.Join(root, "memory", "adaptive", "reports", "experiments", q.ChildID+"-"+q.Subject+"-"+q.Topic+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0644)
}

func lineageIDs(line []adaptive.AdaptiveArtifact) string {
	ids := make([]string, 0, len(line))
	for _, art := range line {
		ids = append(ids, art.ID)
	}
	return strings.Join(ids, " -> ")
}
