package meta

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLessonBundle(t *testing.T) {
	root := t.TempDir()
	writeFiles(root, map[string]string{
		"runs/r1/video.mp4":       "video",
		"runs/r1/validation.json": "{}",
		"runs/r1/preview.html":    "<html>",
		"runs/r1/app.html":        "<html>",
		"runs/r1/screenshot.png":  "png",
	})
	t.Run("valid_manim_bundle", func(t *testing.T) {
		b := LessonBundle{
			SchemaVersion:      LessonBundleSchema,
			ID:                 "bundle-1",
			LessonID:           "lesson-1",
			Kind:               KindManimLesson,
			Status:             "draft",
			LearningObjectives: []string{"multiply fractions"},
			Artifacts: []ArtifactRef{
				{ID: "a-video", Kind: "video", Path: "runs/r1/video.mp4"},
				{ID: "a-validation", Kind: "validation_report", Path: "runs/r1/validation.json"},
				{ID: "a-preview", Kind: "preview_page", Path: "runs/r1/preview.html"},
			},
		}
		if err := ValidateLessonBundle(root, b); err != nil {
			t.Fatalf("valid bundle rejected: %v", err)
		}
	})
	t.Run("missing_video_fails", func(t *testing.T) {
		b := LessonBundle{
			SchemaVersion:      LessonBundleSchema,
			ID:                 "bundle-1",
			LessonID:           "lesson-1",
			Kind:               KindManimLesson,
			Status:             "draft",
			LearningObjectives: []string{"multiply fractions"},
			Artifacts: []ArtifactRef{
				{ID: "a-validation", Kind: "validation_report", Path: "runs/r1/validation.json"},
				{ID: "a-preview", Kind: "preview_page", Path: "runs/r1/preview.html"},
			},
		}
		err := ValidateLessonBundle(root, b)
		if err == nil || !strings.Contains(err.Error(), "missing required artifact kind video") {
			t.Fatalf("missing video error = %v", err)
		}
	})
	t.Run("valid_browser_game_bundle_stable_ids", func(t *testing.T) {
		b := LessonBundle{
			SchemaVersion:      LessonBundleSchema,
			ID:                 "bundle-2",
			LessonID:           "lesson-2",
			Kind:               KindBrowserGameLesson,
			Status:             "accepted",
			LearningObjectives: []string{"sort states of matter"},
			Artifacts: []ArtifactRef{
				{ID: "a-app", Kind: "html_app", Path: "runs/r1/app.html"},
				{ID: "a-preview-url", Kind: "preview_url", URL: "file://" + filepath.Join(root, "runs/r1/preview.html")},
				{ID: "a-shot", Kind: "screenshot", Path: "runs/r1/screenshot.png"},
				{ID: "a-validation", Kind: "validation_report", Path: "runs/r1/validation.json"},
			},
		}
		if err := ValidateLessonBundle(root, b); err != nil {
			t.Fatalf("valid browser bundle rejected: %v", err)
		}
	})
}

func TestArtifactPathGuardRejectsOutsideWorkspace(t *testing.T) {
	err := ValidateArtifactRef(t.TempDir(), ArtifactRef{ID: "a-out", Kind: "log", Path: "../../outside-workspace/file.txt"})
	if err == nil || !strings.Contains(err.Error(), "path guard violation") {
		t.Fatalf("path guard error = %v", err)
	}
}

func TestExtensionContractRejectsModelWeightMutation(t *testing.T) {
	root := t.TempDir()
	writeFiles(root, map[string]string{"workspace/templates/manim_lesson/lesson.py": ""})
	c := ExtensionContract{
		ID:                "create_manim_lesson",
		Profile:           KindManimLesson,
		TemplatePath:      "workspace/templates/manim_lesson",
		Toolchain:         []string{"python", "manimgl", "ffmpeg"},
		RequiredArtifacts: []string{"video", "validation_report", "preview_page"},
		DefaultEvalGate:   "eval:manim_lesson_smoke",
		MutationTargets:   []string{"model_weights"},
	}
	err := ValidateExtensionContract(root, c)
	if err == nil || !strings.Contains(err.Error(), "model weight") {
		t.Fatalf("model weight target error = %v", err)
	}
}
