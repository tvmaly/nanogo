package spikes_test

import (
	"os"
	"strings"
	"testing"
)

func TestPhase22SpikeRecordsDecisionThresholds(t *testing.T) {
	data, err := os.ReadFile("phase22_frame_eval.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{
		"Default fps recommendation",
		"VISION_MODEL",
		"at least 80 percent agreement",
		"False-positive guard",
		"eval.max_cost_usd_per_eval",
		"Default observer: `ext/eval/manual`",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("spike doc missing %q", want)
		}
	}
}

func TestPhase22FixtureSetupDocumentsNonSensitiveFixtures(t *testing.T) {
	data, err := os.ReadFile("phase22_fixture_setup.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{"testdata/phase22/", "Do not check in real child clips", "VISION_MODEL", "FFMPEG"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("fixture setup missing %q", want)
		}
	}
}
