package lessonfactory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tvmaly/nanogo/ext/meta"
	"gopkg.in/yaml.v3"
)

type MicroLessonCompileRequest struct {
	Prompt      string
	ChildID     string
	SkillType   string
	SourcesPath string
}

func (f *Factory) CompileMicroLessonBundle(_ context.Context, req MicroLessonCompileRequest) (meta.LessonBundleV2, error) {
	if req.ChildID == "" {
		req.ChildID = "cross"
	}
	segments := []struct {
		ID         string
		Title      string
		Concept    string
		VideoID    string
		Provenance string
		Fast       bool
	}{
		{"ml-01-the-throw", "The Basic Throw", "A strong straight throw makes every trick possible.", "vid-basic-throw", "model_inferred", true},
		{"ml-02-sleeper", "The Sleeper", "A straight throw lets the yo-yo sleep at the bottom.", "vid-sleeper", "chapters", false},
		{"ml-03-walk-the-dog", "Walk the Dog", "A sleeping yo-yo can roll forward gently.", "vid-walkdog", "chapters", false},
	}
	var micro []meta.MicroLessonV2
	for i, seg := range segments {
		requires := []string(nil)
		if i > 0 {
			requires = []string{segments[i-1].ID}
		}
		ml := meta.MicroLessonV2{
			ID: seg.ID, Title: seg.Title, Concept: seg.Concept, ObjectiveType: meta.ObjectivePhysicalSkill,
			StudentWorkTarget: "Perform " + strings.ToLower(seg.Title) + " with visible control.",
			Requires:          requires,
			Video:             meta.VideoSegment{Provider: "youtube", VideoID: seg.VideoID, StartSeconds: 42, EndSeconds: 130, Provenance: seg.Provenance, SelectedBecause: "Clear beginner demonstration.", ParentCheckRequired: seg.Provenance == "model_inferred"},
			SafetySetup:       meta.SafetySetup{Child: "Stand away from breakables and give your arm room.", Parent: "Use a beginner responsive yo-yo and clear nearby objects."},
			TutorFlow: meta.TutorFlow{
				OpeningPrompt:    "Watch the clip, then try one careful attempt.",
				DiagnosticPrompt: "What did the yo-yo do on your last try?",
				ScaffoldLadder:   []meta.ScaffoldStep{{Level: 0, Move: "wait", Prompt: "Watch your last try."}, {Level: 1, Move: "content_free", Prompt: "What do you notice?"}, {Level: 2, Move: "highlight_feature", Prompt: "Look at your release direction."}},
				ExplanationCard:  meta.ExplanationCard{UseAfter: []string{"student has tried the activity", "student has described one observation"}, Concise: "A straight release helps the yo-yo unwind cleanly.", ActiveCheck: "Try again with your palm facing down."},
				FadeRule:         "After a visual pass, ask for reflection only when configured.",
			},
			Activity:         meta.ActivitySpec{Instructions: "Try " + strings.ToLower(seg.Title) + " five times.", SuccessCriterion: "Yo-yo position and string alignment match the rubric.", Capture: "clip", MaxCaptureSeconds: 20},
			Evaluation:       meta.EvaluationSpec{RubricID: "rb-" + seg.ID},
			LearningEvidence: meta.LearningEvidence{Requires: []meta.EvidenceKind{meta.EvidencePhysicalPerformance}, CompletionRule: meta.CompletionPhysicalPass, ReflectionPrompt: "Optional: what made your best try work?"},
			Advancement:      meta.AdvancementRule{Mode: "linear", MasteryThreshold: 0.8},
		}
		if seg.Fast {
			ml.Evaluation.Sampling = meta.SamplingSpec{FPS: 8, MaxFrames: 24}
		}
		micro = append(micro, ml)
	}
	b := meta.LessonBundleV2{SchemaVersion: meta.LessonBundleSchemaV2, Kind: meta.KindBrowserMicro, ID: "lesson-physical-yoyo-tricks", Title: "Beginner Yo-Yo Tricks", Dashboard: meta.DashboardRef{ChildID: req.ChildID}, MicroLessons: micro}
	if err := meta.ValidateLessonBundleV2(b); err != nil {
		return b, err
	}
	if err := f.writeMicroLessonBundle(b); err != nil {
		return b, err
	}
	return b, nil
}

func (f *Factory) writeMicroLessonBundle(b meta.LessonBundleV2) error {
	base := filepath.Join(f.root, "lessons", "generated", b.ID)
	if err := os.MkdirAll(filepath.Join(base, "rubrics"), 0755); err != nil {
		return err
	}
	data, _ := yaml.Marshal(b)
	if err := os.WriteFile(filepath.Join(base, "lesson.bundle.yaml"), data, 0644); err != nil {
		return err
	}
	var guide strings.Builder
	guide.WriteString("# Parent Guide\n\n## Safety Setup\n")
	for _, ml := range b.MicroLessons {
		guide.WriteString("- " + ml.Title + ": " + ml.SafetySetup.Parent + "\n")
	}
	guide.WriteString("\n## Video Segments\n")
	for _, ml := range b.MicroLessons {
		guide.WriteString(fmt.Sprintf("- %s: %s %d-%d provenance=%s\n", ml.Title, ml.Video.VideoID, ml.Video.StartSeconds, ml.Video.EndSeconds, ml.Video.Provenance))
		if ml.Video.Provenance == "model_inferred" {
			guide.WriteString("  Parent verification required for this inferred segment.\n")
		}
	}
	guide.WriteString("\n## Capture Instructions\nReview safety before recording a short local clip.\n")
	if err := os.WriteFile(filepath.Join(base, "parent_guide.md"), []byte(guide.String()), 0644); err != nil {
		return err
	}
	for _, ml := range b.MicroLessons {
		r := meta.RubricV1{SchemaVersion: meta.RubricSchemaV1, ID: ml.Evaluation.RubricID, MicroLesson: ml.ID, PassRule: meta.CompletionPhysicalPass, Checks: []meta.RubricCheck{{ID: "full-extension", Description: "Yo-yo reaches expected position", Required: true, Critical: true}, {ID: "string-vertical", Description: "String alignment is visible", Required: true}}}
		data, _ := yaml.Marshal(r)
		if err := os.WriteFile(filepath.Join(base, "rubrics", r.ID+".yaml"), data, 0644); err != nil {
			return err
		}
	}
	return nil
}
