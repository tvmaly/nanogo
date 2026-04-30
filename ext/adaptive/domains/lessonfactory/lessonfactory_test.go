package lessonfactory_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/domains/lessonfactory"
	"github.com/tvmaly/nanogo/ext/adaptive/profile"
)

func TestLessonFactoryRegistration(t *testing.T) {
	t.Parallel()
	d, err := adaptive.BuildDomain("lessonfactory", nil)
	if err != nil {
		t.Fatalf("BuildDomain: %v", err)
	}
	want := map[adaptive.ArtifactKind]bool{
		adaptive.ArtifactLessonBundle: true,
		adaptive.ArtifactPathway:      true,
		adaptive.ArtifactRubric:       true,
		adaptive.ArtifactTemplate:     true,
	}
	for _, kind := range d.ArtifactKinds() {
		delete(want, kind)
	}
	if len(want) != 0 {
		t.Fatalf("missing artifact kinds: %#v", want)
	}
}

func TestParseCompleteLessonRequest(t *testing.T) {
	t.Parallel()
	req, warnings, err := lessonfactory.ParseMarkdown("inbox/lessons/magnets.md", completeMarkdown())
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if req.Title != "How magnets work" || req.Subject != "science" || req.Topic != "magnets" || req.AgeHint != 7 {
		t.Fatalf("bad request fields: %#v", req)
	}
	if got := strings.Join(req.Children, ","); got != "cross" {
		t.Fatalf("children = %q", got)
	}
	if !strings.Contains(req.RoughMarkdown, "plays with magnets") {
		t.Fatalf("body was not preserved: %q", req.RoughMarkdown)
	}
}

func TestParseMissingFrontmatterAndValidation(t *testing.T) {
	t.Parallel()
	req, warnings, err := lessonfactory.ParseMarkdown("idea.md", "Teach magnets with paper clips.")
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if req.Title == "" || req.Topic != "magnets" {
		t.Fatalf("best effort parse failed: %#v", req)
	}
	if len(req.Children) != 0 {
		t.Fatalf("parser invented child ID: %#v", req.Children)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected parent-review warnings")
	}

	req.Goal = ""
	req.AgeHint = 2
	req.Materials = []string{"sharp knife"}
	errs := req.Validate()
	for _, field := range []string{"children", "goal", "rough_age_level", "materials"} {
		if !strings.Contains(errs.Error(), field) {
			t.Fatalf("validation did not mention %s: %v", field, errs)
		}
	}
}

func TestCompileLessonBundle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	storeProfile(t, root, "cross", "hands-on demos and short explanations", "memory/adaptive/profiles/cross.json")
	bundle, err := lessonfactory.New(lessonfactory.Config{Root: root, Clock: fixedClock}).CompileBundle(context.Background(), mustRequest(t))
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}
	base := filepath.Join(root, "lessons", "generated", bundle.ID)
	for _, rel := range []string{
		"lesson.yaml", "parent_guide.md", "child_summary.md",
		"child_pathways/cross-default.md", "child_pathways/cross-hands-on.md", "child_pathways/cross-remediation.md",
		"levels/age-6-7.md", "levels/deeper-dive.md", "activities/activity-01.md",
		"assessment/quick-check.md", "assessment/rubric.md",
		"assessment/transfer-questions.md", "assessment/retention-review.md",
		"sources.md", "review.md",
	} {
		if _, err := os.Stat(filepath.Join(base, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	manifest := read(t, filepath.Join(base, "lesson.yaml"))
	for _, s := range []string{"id: lesson-science-magnets", "approval_status: pending", "strategy: default", "strategy: hands_on", "kind: transfer"} {
		if !strings.Contains(manifest, s) {
			t.Fatalf("manifest missing %q:\n%s", s, manifest)
		}
	}
	parent := read(t, filepath.Join(base, "parent_guide.md"))
	for _, s := range []string{"Preparation", "Materials", "Safety Notes", "Expected Misconceptions", "Answer Key"} {
		if !strings.Contains(parent, s) {
			t.Fatalf("parent guide missing %q:\n%s", s, parent)
		}
	}
	defaultPath := read(t, filepath.Join(base, "child_pathways/cross-default.md"))
	if !strings.Contains(defaultPath, "Start with a hands-on investigation") || !strings.Contains(defaultPath, "memory/adaptive/profiles/cross.json") {
		t.Fatalf("default pathway did not use profile pattern:\n%s", defaultPath)
	}
}

func TestMultiChildPathwaysAgeLevelsAndReviewGates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	req := mustRequest(t)
	req.Children = []string{"cross", "older"}
	req.AgeHint = 7
	bundle, err := lessonfactory.New(lessonfactory.Config{Root: root, Clock: fixedClock}).CompileBundle(context.Background(), req)
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}
	if len(bundle.Pathways) != 6 {
		t.Fatalf("pathway count = %d", len(bundle.Pathways))
	}
	base := filepath.Join(root, "lessons", "generated", bundle.ID)
	young := read(t, filepath.Join(base, "levels/age-6-7.md"))
	deep := read(t, filepath.Join(base, "levels/deeper-dive.md"))
	if !strings.Contains(young, "short steps") || !strings.Contains(deep, "field lines") || young == deep {
		t.Fatalf("levels not meaningfully different")
	}

	review := lessonfactory.ReviewBundle(bundle, lessonfactory.ReviewConfig{SourceChecking: true})
	if !review.Assignable {
		t.Fatalf("expected assignable review: %#v", review)
	}
	bundle.Assessments = bundle.Assessments[:2]
	review = lessonfactory.ReviewBundle(bundle, lessonfactory.ReviewConfig{SourceChecking: true})
	if review.Assignable || !strings.Contains(strings.Join(review.Blockers, "\n"), "retention") {
		t.Fatalf("expected retention blocker: %#v", review)
	}
}

func TestApprovalRevisionMutationOutcomeAndArchive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	f := lessonfactory.New(lessonfactory.Config{Root: root, Clock: fixedClock})
	bundle, err := f.CompileBundle(ctx, mustRequest(t))
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}
	if err := f.Assign(ctx, bundle.ID, "cross"); err == nil {
		t.Fatalf("assign without approval succeeded")
	}
	if err := f.RecordParentReview(ctx, bundle.ID, lessonfactory.ParentReview{Approved: true, Notes: "ok"}); err != nil {
		t.Fatalf("RecordParentReview: %v", err)
	}
	if err := f.Assign(ctx, bundle.ID, "cross"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "lessons", "queues", "cross.jsonl")); err != nil {
		t.Fatalf("queue missing: %v", err)
	}

	rev, err := f.Revise(ctx, bundle.ID, "make it shorter")
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	if rev.ID == bundle.ID || rev.ParentID != bundle.ID {
		t.Fatalf("bad revision lineage: %#v", rev)
	}
	mut, err := f.MutateTemplate(ctx, "hands-on-science", "shorter setup")
	if err != nil {
		t.Fatalf("MutateTemplate: %v", err)
	}
	if mut.ParentID == "" || mut.Kind != adaptive.ArtifactTemplate {
		t.Fatalf("bad template mutation: %#v", mut)
	}
	out := f.RecordChildOutcome(ctx, bundle.ID, lessonfactory.ChildOutcome{
		ChildID: "cross", QuizScore: 0.8, HintCount: 1, ParentRating: 4, Engagement: 0.9, TransferSuccess: true, Retention: 0.7,
	})
	if out.MasteryGain == 0 || out.TransferScore == 0 || out.CombinedScore == 0 {
		t.Fatalf("bad outcome mapping: %#v", out)
	}
	ar, err := archive.New(root)
	if err != nil {
		t.Fatal(err)
	}
	arts, err := ar.Top(ctx, archive.Query{ChildID: "cross", Subject: "science", Topic: "magnets", IncludeFailures: true}, 1)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(arts) != 1 || arts[0].ID != bundle.ID {
		t.Fatalf("archive top = %#v", arts)
	}
	next, err := f.CompileBundle(ctx, mustRequest(t))
	if err != nil {
		t.Fatalf("second CompileBundle: %v", err)
	}
	review := read(t, filepath.Join(root, "lessons", "generated", next.ID, "review.md"))
	if !strings.Contains(review, "prior outcomes") {
		t.Fatalf("future lesson was not archive-informed:\n%s", review)
	}
}

func TestResearchPolicySourcesAndFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := lessonfactory.Config{Root: root, Clock: fixedClock, Research: lessonfactory.ResearchConfig{
		Enabled: true, TrustedDomainsOnly: true, TrustedDomains: []string{"nasa.gov", "si.edu"}, MaxResults: 5, MaxTotalResults: 15, SearchContextSize: "medium",
	}}
	f := lessonfactory.New(cfg)
	req := mustRequest(t)
	req.Research.TrustedDomains = []string{"nationalgeographic.com"}
	tool, err := f.ResearchToolFor(req)
	if err != nil {
		t.Fatalf("ResearchToolFor: %v", err)
	}
	data, _ := json.Marshal(tool)
	for _, s := range []string{"openrouter:web_search", "nasa.gov", "si.edu", "nationalgeographic.com", "max_total_results"} {
		if !strings.Contains(string(data), s) {
			t.Fatalf("research tool missing %q: %s", s, data)
		}
	}
	bundle, err := f.CompileBundle(context.Background(), req)
	if err != nil {
		t.Fatalf("CompileBundle: %v", err)
	}
	base := filepath.Join(root, "lessons", "generated", bundle.ID)
	sources := read(t, filepath.Join(base, "sources.md"))
	if !strings.Contains(sources, "Retrieved: 2026-01-02") || !strings.Contains(sources, "nasa.gov") {
		t.Fatalf("bad sources:\n%s", sources)
	}
	review := read(t, filepath.Join(base, "review.md"))
	if !strings.Contains(review, "Trusted domains") || !strings.Contains(review, "nationalgeographic.com") {
		t.Fatalf("review missing research policy:\n%s", review)
	}

	req.Research.Enabled = boolPtr(false)
	disabled, err := f.CompileBundle(context.Background(), req)
	if err != nil {
		t.Fatalf("disabled CompileBundle: %v", err)
	}
	if tool, err := f.ResearchToolFor(req); err != nil || tool != nil {
		t.Fatalf("disabled research attached tool: %#v %v", tool, err)
	}
	if !strings.Contains(read(t, filepath.Join(root, "lessons", "generated", disabled.ID, "sources.md")), "No web research performed") {
		t.Fatalf("disabled sources note missing")
	}
	if _, err := lessonfactory.NewCheckedConfig(lessonfactory.Config{Research: lessonfactory.ResearchConfig{Enabled: true, TrustedDomainsOnly: true}}); err == nil {
		t.Fatalf("empty trusted domain config did not fail")
	}
}

func TestInboxWorkflowAndExtensionBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox", "lessons")
	if err := os.MkdirAll(inbox, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(inbox, "magnets.md")
	if err := os.WriteFile(src, []byte(completeMarkdown()), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := lessonfactory.New(lessonfactory.Config{Root: root, Clock: fixedClock}).ProcessInboxFile(context.Background(), src)
	if err != nil {
		t.Fatalf("ProcessInboxFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "lessons", "generated", bundle.ID)); err != nil {
		t.Fatalf("bundle missing: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source not preserved: %v", err)
	}
}

func completeMarkdown() string {
	return `---
title: How magnets work
subject: science
topic: magnets
children: [cross]
rough_age_level: 7
goal: Help him understand attraction, repulsion, and magnetic fields
materials:
  - magnets
  - paper clips
  - paper
preferences:
  - make it hands-on
research:
  trusted_domains:
    - nationalgeographic.com
---

I want a lesson where he plays with magnets and learns why some things stick and some do not.
`
}

func mustRequest(t *testing.T) lessonfactory.LessonRequest {
	t.Helper()
	req, _, err := lessonfactory.ParseMarkdown("magnets.md", completeMarkdown())
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func storeProfile(t *testing.T, root, childID, style, source string) {
	t.Helper()
	ps, err := profile.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := ps.Propose(context.Background(), profile.Change{ChildID: childID, Field: "science_style", Proposed: style})
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.Resolve(context.Background(), ch.ID, profile.Approved, ""); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fixedClock() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

func boolPtr(v bool) *bool { return &v }
