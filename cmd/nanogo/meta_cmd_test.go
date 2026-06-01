package main

import (
	"strings"
	"testing"
)

func TestRunMetaLessonCreateManimReportsVideoAndPreview(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runMetaCmd([]string{"lesson", "create", "--kind", "manim_lesson", "--prompt", "Teach multiplying fractions to a 9-year-old", "--runner", "fake"}, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"lesson_id:", "run_id:", "decision: accepted", "eligible_for_promotion: true", "video_path:", "preview_path:", "bundle_path:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunMetaLessonCreateBrowserGameReportsPreviewAndValidation(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runMetaCmd([]string{"lesson", "create", "--kind", "browser_game_lesson", "--prompt", "Teach states of matter with a drag-and-drop sorting game", "--runner", "fake"}, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"decision: accepted", "preview_path:", "preview_url:", "validation_report:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunMetaLessonCreateRejectsInvalidKind(t *testing.T) {
	err := runMetaCmd([]string{"lesson", "create", "--kind", "bad", "--prompt", "Teach states of matter", "--runner", "fake"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "invalid lesson kind") {
		t.Fatalf("invalid kind error = %v", err)
	}
}
