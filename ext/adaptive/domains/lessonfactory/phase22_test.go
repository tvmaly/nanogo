package lessonfactory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/adaptive/domains/lessonfactory"
)

func TestCompilePhysicalMicroLessonBundleLinearChainRubricsAndGuide(t *testing.T) {
	root := t.TempDir()
	f := lessonfactory.New(lessonfactory.Config{Root: root})
	b, err := f.CompileMicroLessonBundle(context.Background(), lessonfactory.MicroLessonCompileRequest{Prompt: "teach my 7 year old yo-yo tricks", SkillType: "physical", ChildID: "cross"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.MicroLessons) != 3 || len(b.MicroLessons[0].Requires) != 0 || b.MicroLessons[1].Requires[0] != b.MicroLessons[0].ID {
		t.Fatalf("micro lesson chain = %+v", b.MicroLessons)
	}
	if b.MicroLessons[0].ObjectiveType != "physical_skill" || b.MicroLessons[0].LearningEvidence.CompletionRule != "physical_pass" {
		t.Fatalf("physical evidence = %+v", b.MicroLessons[0])
	}
	if b.MicroLessons[0].Evaluation.Sampling.FPS < 8 {
		t.Fatalf("fast action sampling missing: %+v", b.MicroLessons[0].Evaluation.Sampling)
	}
	base := filepath.Join(root, "lessons", "generated", b.ID)
	for _, ml := range b.MicroLessons {
		data, err := os.ReadFile(filepath.Join(base, "rubrics", ml.Evaluation.RubricID+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "schema_version: eval.rubric.v1") || !strings.Contains(string(data), "pass_rule: physical_pass") {
			t.Fatalf("bad rubric: %s", data)
		}
	}
	guide, err := os.ReadFile(filepath.Join(base, "parent_guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Safety Setup", "provenance=model_inferred", "Parent verification required", "Capture Instructions"} {
		if !strings.Contains(string(guide), want) {
			t.Fatalf("parent guide missing %q: %s", want, guide)
		}
	}
}
