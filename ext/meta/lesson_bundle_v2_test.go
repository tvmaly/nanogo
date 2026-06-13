package meta_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/meta"
)

func TestV1BundleNormalizesToSingleMicroLesson(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.yaml")
	if err := os.WriteFile(path, []byte(`schema_version: meta.lesson_bundle.v1
id: candidate-1
lesson_id: lesson-1
kind: manim_lesson
title: Magnets
status: draft
learning_objectives: [Understand magnets]
promotion: {}
`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := meta.LoadLessonBundleV2(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != meta.LessonBundleSchemaV2 || len(got.MicroLessons) != 1 {
		t.Fatalf("normalized = %+v", got)
	}
}

func TestLessonBundleV2RejectsCyclesAndMixedPhysicalOnly(t *testing.T) {
	b := validBundle()
	b.MicroLessons[0].Requires = []string{"ml-02"}
	b.MicroLessons[1].Requires = []string{"ml-01"}
	b.MicroLessons[1].ObjectiveType = meta.ObjectiveMixed
	b.MicroLessons[1].LearningEvidence = meta.LearningEvidence{
		Requires: []meta.EvidenceKind{meta.EvidencePhysicalPerformance}, CompletionRule: meta.CompletionPhysicalPass,
	}
	err := meta.ValidateLessonBundleV2(b)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"cycle", "mixed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestPhysicalSkillMayAdvanceOnPhysicalPerformanceOnly(t *testing.T) {
	if err := meta.ValidateLessonBundleV2(validBundle()); err != nil {
		t.Fatal(err)
	}
}

func TestRubricRejectsFreeTextPassRule(t *testing.T) {
	err := meta.ValidateRubricV1(meta.RubricV1{
		SchemaVersion: meta.RubricSchemaV1,
		ID:            "rb-1",
		MicroLesson:   "ml-1",
		PassRule:      "model decides holistically",
		Checks:        []meta.RubricCheck{{ID: "c1", Required: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "pass_rule") {
		t.Fatalf("expected pass_rule error, got %v", err)
	}
}

func validBundle() meta.LessonBundleV2 {
	ml := func(id string, requires []string) meta.MicroLessonV2 {
		return meta.MicroLessonV2{
			ID: id, Title: id, Concept: "throw straight", ObjectiveType: meta.ObjectivePhysicalSkill,
			StudentWorkTarget: "Perform the throw.", Requires: requires,
			Video:            meta.VideoSegment{Provider: "youtube", VideoID: "abc123", StartSeconds: 1, EndSeconds: 10, Provenance: "chapters", SelectedBecause: "clear"},
			SafetySetup:      meta.SafetySetup{Child: "clear space", Parent: "check area"},
			TutorFlow:        meta.TutorFlow{OpeningPrompt: "watch", DiagnosticPrompt: "what happened?", ScaffoldLadder: []meta.ScaffoldStep{{Level: 0, Move: "wait"}, {Level: 1, Move: "hint"}}, ExplanationCard: meta.ExplanationCard{UseAfter: []string{"try"}, ActiveCheck: "try again"}},
			Activity:         meta.ActivitySpec{Instructions: "throw", SuccessCriterion: "vertical string", Capture: "clip", MaxCaptureSeconds: 20},
			Evaluation:       meta.EvaluationSpec{RubricID: "rb-throw-v1"},
			LearningEvidence: meta.LearningEvidence{Requires: []meta.EvidenceKind{meta.EvidencePhysicalPerformance}, CompletionRule: meta.CompletionPhysicalPass},
			Advancement:      meta.AdvancementRule{Mode: "linear", MasteryThreshold: 0.8},
		}
	}
	return meta.LessonBundleV2{SchemaVersion: meta.LessonBundleSchemaV2, Kind: meta.KindBrowserMicro, ID: "lesson-yoyo", MicroLessons: []meta.MicroLessonV2{ml("ml-01", nil), ml("ml-02", []string{"ml-01"})}}
}
