package lesson_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/modules/lesson"
)

func TestLessonListStartNonceEventsAndAdvance(t *testing.T) {
	root := t.TempDir()
	svc := lesson.New(lesson.Config{Root: root, Nonce: func() string { return "nonce-1" }, Now: fixedTime})
	bundle := testBundle()
	list := svc.List(context.Background(), "cross", []lesson.Bundle{
		bundle,
		{ID: "draft", ChildID: "cross", Approved: false, Promoted: true, Assigned: true},
	})
	if len(list) != 1 || list[0].ID != "lesson-yoyo" {
		t.Fatalf("list = %+v", list)
	}
	sess, err := svc.Start(context.Background(), bundle, "cross")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Nonce == "" || sess.CurrentMicroID != "ml-01" {
		t.Fatalf("session = %+v", sess)
	}
	if err := svc.Event(context.Background(), sess.ID, lesson.Event{Type: "progress", Nonce: "bad"}); err == nil {
		t.Fatal("expected invalid nonce error")
	}
	if err := svc.Event(context.Background(), sess.ID, lesson.Event{Type: "reflection", Nonce: "nonce-1", AttemptID: "a1"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "memory", "adaptive", "tutorruntime", "turns.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lesson.turn.v1", "lesson_session_id", "actor_role", "reflection"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("turn record missing %q: %s", want, data)
		}
	}
	denied := svc.Advance(context.Background(), sess, bundle, nil)
	if denied.Advanced || denied.RemediationRef == "" {
		t.Fatalf("expected remediation denial: %+v", denied)
	}
	advanced := svc.Advance(context.Background(), sess, bundle, map[string]lesson.Verdict{"ml-01": {VisualPass: true, TrustRampPermits: true}})
	if !advanced.Advanced || advanced.CurrentMicroLessonID != "ml-02" {
		t.Fatalf("advance = %+v", advanced)
	}
}

func TestEvidenceGatesMixedAndDeepAgainstVisualOnly(t *testing.T) {
	physical := lesson.MicroLesson{ObjectiveType: lesson.ObjectivePhysicalSkill, LearningEvidence: lesson.LearningEvidence{Requires: []lesson.EvidenceKind{lesson.EvidencePhysicalPerformance}}}
	if !lesson.EvidenceSatisfied(physical, lesson.Verdict{VisualPass: true, TrustRampPermits: true}) {
		t.Fatal("physical skill should pass on visual evidence")
	}
	mixed := lesson.MicroLesson{ObjectiveType: lesson.ObjectiveMixed, LearningEvidence: lesson.LearningEvidence{Requires: []lesson.EvidenceKind{lesson.EvidencePhysicalPerformance, lesson.EvidenceReflection}}}
	if lesson.EvidenceSatisfied(mixed, lesson.Verdict{VisualPass: true, TrustRampPermits: true}) {
		t.Fatal("mixed objective should not pass without reflection")
	}
}

func TestDashboardReadyRecords(t *testing.T) {
	root := t.TempDir()
	svc := lesson.New(lesson.Config{Root: root, Now: fixedTime})
	if err := svc.RecordReviewItem(lesson.ReviewItem{ID: "review-1", ChildID: "cross", LessonID: "lesson-yoyo", MicroLessonID: "ml-01", Reason: "trust ramp"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordProgress(lesson.ProgressRow{ChildID: "cross", LessonID: "lesson-yoyo", MicroLessonID: "ml-01", Mastery: 0.8}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordNextAction(lesson.NextAction{ChildID: "cross", LessonID: "lesson-yoyo", Action: "continue", Reason: "passed"}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"review_items.jsonl", "progress.jsonl", "next_actions.jsonl"} {
		data, err := os.ReadFile(filepath.Join(root, "memory", "lessons", rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "schema_version") || !strings.Contains(string(data), "cross") {
			t.Fatalf("%s missing dashboard fields: %s", rel, data)
		}
	}
}

func testBundle() lesson.Bundle {
	return lesson.Bundle{ID: "lesson-yoyo", ChildID: "cross", Approved: true, Promoted: true, Assigned: true, MicroLessons: []lesson.MicroLesson{
		{ID: "ml-01", ObjectiveType: lesson.ObjectivePhysicalSkill, LearningEvidence: lesson.LearningEvidence{Requires: []lesson.EvidenceKind{lesson.EvidencePhysicalPerformance}, CompletionRule: lesson.CompletionPhysicalPass}},
		{ID: "ml-02", Requires: []string{"ml-01"}, ObjectiveType: lesson.ObjectiveMixed, LearningEvidence: lesson.LearningEvidence{Requires: []lesson.EvidenceKind{lesson.EvidencePhysicalPerformance, lesson.EvidenceReflection}, CompletionRule: lesson.CompletionAllRequiredEvidence}},
	}}
}

func fixedTime() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
