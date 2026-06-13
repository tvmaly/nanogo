package openrouter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/eval/openrouter"
)

func TestOpenRouterVisionRequestShapeIsFrameOnly(t *testing.T) {
	obs := openrouter.New(openrouter.Config{Model: "vision/test"})
	got, err := obs.ObserveActivity(context.Background(), contracts.ActivityObservationRequest{FrameRefs: []contracts.ArtifactRef{{URI: "data:image/jpeg;base64,abc", Kind: "frame"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"vision/test", "image_url", "json_object", "Do not decide mastery"} {
		if !strings.Contains(got.RawRef, want) {
			t.Fatalf("request missing %q: %s", want, got.RawRef)
		}
	}
}
