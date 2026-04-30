package tutorruntime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/domains/lessonfactory"
	"github.com/tvmaly/nanogo/ext/adaptive/domains/tutorruntime"
	"gopkg.in/yaml.v3"
)

func TestTutorRuntimeRegistration(t *testing.T) {
	d, err := adaptive.BuildDomain("tutorruntime", nil)
	if err != nil {
		t.Fatalf("BuildDomain: %v", err)
	}
	if d.Name() != "tutorruntime" {
		t.Fatalf("Name = %q", d.Name())
	}
	kinds := map[adaptive.ArtifactKind]bool{}
	for _, k := range d.ArtifactKinds() {
		kinds[k] = true
	}
	for _, want := range []adaptive.ArtifactKind{adaptive.ArtifactTutorPolicy, adaptive.ArtifactRubric, adaptive.ArtifactPrompt, adaptive.ArtifactSkill} {
		if !kinds[want] {
			t.Fatalf("missing artifact kind %q in %v", want, d.ArtifactKinds())
		}
	}
}

func TestTutorPolicyRoundtrip(t *testing.T) {
	policies, err := tutorruntime.LoadInitialPolicies()
	if err != nil {
		t.Fatalf("LoadInitialPolicies: %v", err)
	}
	want := []string{"socratic-guide", "worked-example-first", "hands-on-remediation", "visual-analogy", "story-explanation", "retrieval-practice", "challenge-mode", "gentle-coach"}
	byID := map[string]tutorruntime.TutorPolicy{}
	for _, p := range policies {
		if err := p.Validate(); err != nil {
			t.Fatalf("policy %q invalid: %v", p.ID, err)
		}
		data, err := yaml.Marshal(p)
		if err != nil {
			t.Fatalf("marshal yaml: %v", err)
		}
		var round tutorruntime.TutorPolicy
		if err := yaml.Unmarshal(data, &round); err != nil {
			t.Fatalf("unmarshal yaml: %v", err)
		}
		if round.ID != p.ID || round.Strategy != p.Strategy || round.MaxHints != p.MaxHints {
			t.Fatalf("roundtrip lost fields: %#v -> %#v", p, round)
		}
		byID[p.ID] = round
	}
	for _, id := range want {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing initial policy %q", id)
		}
	}
}

func TestPolicyFallbackOrder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rt := newFixtureRuntime(t, root)
	seedPolicyEvidence(t, root)

	cases := []struct {
		name string
		req  tutorruntime.PolicyRequest
		want string
	}{
		{"child subject topic", tutorruntime.PolicyRequest{ChildID: "cross", Subject: "science", Topic: "magnets"}, "visual-analogy"},
		{"child subject", tutorruntime.PolicyRequest{ChildID: "cross", Subject: "science", Topic: "plants"}, "worked-example-first"},
		{"child wide", tutorruntime.PolicyRequest{ChildID: "cross", Subject: "math", Topic: "fractions"}, "gentle-coach"},
		{"global topic", tutorruntime.PolicyRequest{ChildID: "rowan", Subject: "science", Topic: "magnets"}, "story-explanation"},
		{"default", tutorruntime.PolicyRequest{ChildID: "rowan", Subject: "math", Topic: "fractions"}, "socratic-guide"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rt.SelectPolicy(ctx, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if got.Policy.ID != tc.want {
				t.Fatalf("policy = %q, want %q; rationale=%s", got.Policy.ID, tc.want, got.Rationale)
			}
		})
	}
}

func TestParentPinnedPolicyAndExploration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rt := newFixtureRuntime(t, root)
	seedPolicyEvidence(t, root)
	if err := rt.PinPolicy(ctx, "cross", "science", "magnets", "gentle-coach"); err != nil {
		t.Fatal(err)
	}
	pinned, err := rt.SelectPolicy(ctx, tutorruntime.PolicyRequest{ChildID: "cross", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Policy.ID != "gentle-coach" || !strings.Contains(pinned.Rationale, "parent-pinned") {
		t.Fatalf("parent pin not honored: %#v", pinned)
	}

	explore := newFixtureRuntime(t, t.TempDir(), func(c *tutorruntime.Config) {
		c.Epsilon = 1
		c.Seed = 7
	})
	seedPolicyEvidence(t, explore.Root())
	first, err := explore.SelectPolicy(ctx, tutorruntime.PolicyRequest{ChildID: "cross", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Policy.ID == "visual-analogy" || !strings.Contains(first.Rationale, "exploration") {
		t.Fatalf("expected reproducible exploration away from winner, got %#v", first)
	}

	noExplore := newFixtureRuntime(t, t.TempDir(), func(c *tutorruntime.Config) { c.Epsilon = 0 })
	seedPolicyEvidence(t, noExplore.Root())
	winner, err := noExplore.SelectPolicy(ctx, tutorruntime.PolicyRequest{ChildID: "cross", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if winner.Policy.ID != "visual-analogy" {
		t.Fatalf("epsilon=0 should use winner, got %q", winner.Policy.ID)
	}
}

func TestSessionTurnGradeMasteryRetentionAndRemediation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	rt := newFixtureRuntime(t, t.TempDir(), func(c *tutorruntime.Config) {
		c.Clock = func() time.Time { return now }
		c.HintThreshold = 2
		c.AttemptThreshold = 3
		c.RequireTransferForMastered = true
		c.RequireRetentionForMastered = true
	})
	session, err := rt.StartSession(ctx, tutorruntime.StartSessionRequest{ChildID: "cross", LessonID: "magnets-demo", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.ID == "" || session.PolicyID == "" || session.StartedAt.IsZero() {
		t.Fatalf("bad session: %#v", session)
	}
	if !strings.Contains(session.PolicyRationale, "default") {
		t.Fatalf("missing rationale: %#v", session)
	}

	turn := tutorruntime.TutorTurnOutcome{SessionID: session.ID, ChildID: "cross", PolicyID: session.PolicyID, LessonID: "magnets-demo", Subject: "science", Topic: "magnets", QuestionID: "q1", Correct: true, HintCount: 1, Attempts: 1, TimeSeconds: 22, EngagementScore: 0.8, FrustrationScore: 0.1, TransferSuccess: true, Notes: "explained poles"}
	if err := rt.RecordTurn(ctx, turn); err != nil {
		t.Fatalf("RecordTurn: %v", err)
	}
	turn.Correct = false
	turn.HintCount = 3
	turn.Attempts = 3
	if err := rt.RecordTurn(ctx, turn); err != nil {
		t.Fatalf("RecordTurn retry: %v", err)
	}
	turns, err := rt.Turns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want append-only evidence", len(turns))
	}

	grade := rt.GradeAnswer(tutorruntime.GradeRequest{Answer: "North and south poles attract.", Expected: "opposite poles attract", RubricItemID: "rubric-poles"})
	if !grade.Correct || grade.PartialCredit != 1 || grade.RubricItemID != "rubric-poles" || grade.Feedback == "" {
		t.Fatalf("unexpected grade: %#v", grade)
	}
	mis := rt.GradeAnswer(tutorruntime.GradeRequest{Answer: "magnets stick because of glue", Expected: "magnetic fields", RubricItemID: "rubric-fields"})
	if mis.Misconception == "" || mis.Correct {
		t.Fatalf("expected misconception grade: %#v", mis)
	}

	state := rt.UpdateMastery(tutorruntime.MasteryState{Level: tutorruntime.Introduced}, tutorruntime.MasteryEvidence{Correct: true, HintCount: 0, TransferSuccess: false})
	if state.Level != tutorruntime.Practicing {
		t.Fatalf("level = %s", state.Level)
	}
	state = rt.UpdateMastery(tutorruntime.MasteryState{Level: tutorruntime.Practicing}, tutorruntime.MasteryEvidence{Correct: true, HintCount: 4, TransferSuccess: true, RetentionPassed: true})
	if state.Level == tutorruntime.NearlyMastered || state.Level == tutorruntime.Mastered {
		t.Fatalf("excessive hints should not advance high mastery: %s", state.Level)
	}
	state = rt.UpdateMastery(tutorruntime.MasteryState{Level: tutorruntime.NearlyMastered}, tutorruntime.MasteryEvidence{Correct: true, HintCount: 0, TransferSuccess: true, RetentionPassed: false})
	if state.Level != tutorruntime.NearlyMastered {
		t.Fatalf("retention required for mastered, got %s", state.Level)
	}
	state = rt.UpdateMastery(tutorruntime.MasteryState{Level: tutorruntime.Mastered}, tutorruntime.MasteryEvidence{RetentionCheck: true, RetentionPassed: false})
	if state.Level != tutorruntime.NeedsReview {
		t.Fatalf("failed retention should need review, got %s", state.Level)
	}

	items, err := rt.ScheduleRetention(ctx, tutorruntime.RetentionRequest{ChildID: "cross", Topic: "magnets", LessonID: "magnets-demo", QuestionIDs: []string{"r1"}, CompletedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].DueAt.Sub(now) != 24*time.Hour {
		t.Fatalf("bad retention items: %#v", items)
	}

	rec := rt.RecommendRemediation(tutorruntime.RemediationInput{HintCount: 2, Attempts: 1})
	if rec.Action == "" || !rec.BlocksMastery {
		t.Fatalf("expected hint remediation: %#v", rec)
	}
	switched, err := rt.MaybeSwitchStrategy(ctx, session.ID, turn)
	if err != nil {
		t.Fatal(err)
	}
	if switched.NewPolicyID == "" || switched.NewPolicyID == switched.OldPolicyID {
		t.Fatalf("expected strategy switch: %#v", switched)
	}
	gap := rt.RecommendRemediation(tutorruntime.RemediationInput{MissedConcept: "magnetic fields", Prerequisite: "push and pull"})
	if !gap.BlocksMastery || !strings.Contains(gap.Reason, "prerequisite") {
		t.Fatalf("expected prerequisite gap: %#v", gap)
	}
}

func TestProfileScoringMutationApprovalSummaryToolsAndLessonIntegration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rt := newFixtureRuntime(t, root, func(c *tutorruntime.Config) {
		c.RequireParentApprovalForProfileInference = true
		c.RequireParentApprovalForMajorPolicyChange = true
		c.HomeschoolMode = true
	})
	if err := rt.RecordMisconception(ctx, "cross", "magnets", "fields are glue"); err != nil {
		t.Fatal(err)
	}
	pending, err := rt.PendingProfileChanges(ctx, "cross")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].State != "pending" {
		t.Fatalf("expected pending profile change: %#v", pending)
	}

	a := tutorruntime.SessionOutcome{SessionID: "s1", ChildID: "cross", PolicyID: "visual-analogy", CorrectRate: 0.8, TransferScore: 1, RetentionScore: 0.7, EngagementScore: 0.9, FrustrationScore: 0.1, TimeToMasteryMin: 15, ParentRating: 4}
	b := a
	b.PolicyID = "socratic-guide"
	b.EngagementScore = 0.5
	b.FrustrationScore = 0.8
	evalA := rt.EvaluateSessionOutcome(a)
	evalB := rt.EvaluateSessionOutcome(b)
	if evalA.CombinedScore <= evalB.CombinedScore {
		t.Fatalf("engagement/frustration should affect score: %#v <= %#v", evalA, evalB)
	}
	if evalA.MasteryGain == 0 || evalA.TransferScore == 0 || evalA.ParentRating == 0 {
		t.Fatalf("missing eval fields: %#v", evalA)
	}

	parent := tutorruntime.TutorPolicy{ID: "gentle-coach", Strategy: "gentle_coach", Subject: "science", Topic: "magnets", MaxHints: 3}
	mutated, err := rt.MutatePolicy(ctx, parent, adaptive.MutationGoal{Improve: []string{"reduce frustration"}, Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if mutated.ID == parent.ID || mutated.ParentID != parent.ID {
		t.Fatalf("bad mutation: %#v", mutated)
	}
	if _, err := os.Stat(filepath.Join(root, "memory", "adaptive", "artifacts", "tutor_policies", mutated.ID+".yaml")); err != nil {
		t.Fatalf("mutated policy not written: %v", err)
	}

	activation, err := rt.ProposePolicyChange(ctx, "cross", "gentle-coach", "challenge-mode")
	if err != nil {
		t.Fatal(err)
	}
	if activation.Active || !activation.RequiresParentApproval {
		t.Fatalf("major change should require approval: %#v", activation)
	}

	session, err := rt.StartSession(ctx, tutorruntime.StartSessionRequest{ChildID: "cross", LessonID: "magnets-demo", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	_ = rt.RecordTurn(ctx, tutorruntime.TutorTurnOutcome{SessionID: session.ID, ChildID: "cross", PolicyID: session.PolicyID, LessonID: "magnets-demo", Subject: "science", Topic: "magnets", QuestionID: "q1", Correct: true, HintCount: 1, Attempts: 1, EngagementScore: 0.8, FrustrationScore: 0.1, TransferSuccess: true, Notes: "understood poles"})
	summary, err := rt.SummarizeSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"worked on", "understood", "struggled", "hints", "misconceptions", "mastery", "next step", "parent action"} {
		if !strings.Contains(strings.ToLower(summary.Markdown), text) {
			t.Fatalf("summary missing %q:\n%s", text, summary.Markdown)
		}
	}
	evidence, err := rt.HomeschoolEvidence(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Subject != "science" || evidence.QuestionsAttempted != 1 || evidence.SummaryPath == "" {
		t.Fatalf("bad evidence: %#v", evidence)
	}

	source, err := coretools.Build("tutorruntime", mustJSON(t, tutorruntime.Config{Root: root}))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := source.Tools(ctx, coretools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	for _, name := range []string{"tutorruntime_select_policy", "tutorruntime_record_turn", "tutorruntime_grade_answer", "tutorruntime_update_mastery", "tutorruntime_detect_misconception", "tutorruntime_recommend_remediation", "tutorruntime_schedule_review", "tutorruntime_summarize_session"} {
		if !names[name] {
			t.Fatalf("missing tool %q", name)
		}
	}

	lf := lessonfactory.New(lessonfactory.Config{Root: root})
	lesson, err := lf.CompileBundle(ctx, lessonfactory.LessonRequest{Title: "How magnets work", Subject: "science", Topic: "magnets", Children: []string{"cross"}, AgeHint: 7, Goal: "learn attraction and repulsion", Materials: []string{"magnets"}, RoughMarkdown: "Explore magnets."})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := rt.StartSession(ctx, tutorruntime.StartSessionRequest{ChildID: "cross", LessonID: lesson.ID})
	if err != nil {
		t.Fatal(err)
	}
	if linked.Subject != "science" || linked.Topic != "magnets" || linked.LessonArtifactID != lesson.ID {
		t.Fatalf("lesson bundle not linked: %#v", linked)
	}
	_ = rt.RecordTurn(ctx, tutorruntime.TutorTurnOutcome{SessionID: linked.ID, ChildID: "cross", PolicyID: linked.PolicyID, LessonID: lesson.ID, Subject: "science", Topic: "magnets", QuestionID: "q1", Correct: true, EngagementScore: 0.7, FrustrationScore: 0.2})
	if err := rt.ArchiveSessionOutcome(ctx, linked.ID); err != nil {
		t.Fatal(err)
	}
	outcomes, err := archiveOutcomes(root)
	if err != nil {
		t.Fatal(err)
	}
	var policyOutcome, lessonOutcome bool
	for _, out := range outcomes {
		policyOutcome = policyOutcome || strings.Contains(out.ArtifactID, linked.PolicyID)
		lessonOutcome = lessonOutcome || out.ArtifactID == lesson.ID
	}
	if !policyOutcome || !lessonOutcome {
		t.Fatalf("expected policy and lesson outcomes, got %#v", outcomes)
	}
}

func TestDomainEvaluateCompileMutate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	d, err := adaptive.BuildDomain("tutorruntime", mustJSON(t, tutorruntime.Config{Root: root}))
	if err != nil {
		t.Fatal(err)
	}
	arts, err := d.Compile(ctx, adaptive.CompileRequest{ChildID: "cross", Subject: "science", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) == 0 || arts[0].Kind != adaptive.ArtifactTutorPolicy {
		t.Fatalf("bad artifacts: %#v", arts)
	}
	out, err := d.Evaluate(ctx, arts[0], adaptive.Attempt{ID: "a1", ChildID: "cross", Observations: map[string]any{"correct_rate": 0.9, "engagement": 0.8, "frustration": 0.1, "transfer": 1.0, "retention": 0.7}})
	if err != nil {
		t.Fatal(err)
	}
	if out.CombinedScore == 0 || !out.Correctness {
		t.Fatalf("bad eval: %#v", out)
	}
	muts, err := d.Mutate(ctx, arts[0], adaptive.MutationGoal{Improve: []string{"more visual"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(muts) != 1 || muts[0].ParentID != arts[0].ID {
		t.Fatalf("bad mutation: %#v", muts)
	}
}

func newFixtureRuntime(t *testing.T, root string, opts ...func(*tutorruntime.Config)) *tutorruntime.Runtime {
	t.Helper()
	cfg := tutorruntime.Config{Root: root, DefaultPolicyID: "socratic-guide", Seed: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	rt, err := tutorruntime.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func seedPolicyEvidence(t *testing.T, root string) {
	t.Helper()
	ctx := context.Background()
	ar, err := archive.New(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	records := []struct {
		id, child, subject, topic, strategy string
		score                               float64
	}{
		{"visual-analogy", "cross", "science", "magnets", "visual_analogy", 0.95},
		{"worked-example-first", "cross", "science", "", "worked_example", 0.85},
		{"gentle-coach", "cross", "", "", "gentle_coach", 0.75},
		{"story-explanation", "", "", "magnets", "story", 0.65},
	}
	for _, r := range records {
		art := adaptive.AdaptiveArtifact{ID: r.id, Kind: adaptive.ArtifactTutorPolicy, ChildID: r.child, Subject: r.subject, Topic: r.topic, Strategy: r.strategy, Version: 1, CreatedAt: now}
		if err := ar.AddArtifact(ctx, art); err != nil {
			t.Fatal(err)
		}
		if err := ar.AddOutcome(ctx, adaptive.AdaptiveEvalResult{ArtifactID: r.id, ChildID: r.child, Correctness: true, CombinedScore: r.score, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func archiveOutcomes(root string) ([]adaptive.AdaptiveEvalResult, error) {
	ar, err := archive.New(root)
	if err != nil {
		return nil, err
	}
	return ar.Outcomes(context.Background())
}
