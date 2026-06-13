package ffmpeg_test

import (
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/media/ffmpeg"
)

func TestSamplerCommandUsesDefaultStrideAndBounds(t *testing.T) {
	cmd, err := (ffmpeg.Sampler{}).Command(ffmpeg.Request{Input: "capture.webm", OutputDir: "/tmp/frames"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd, " ")
	for _, want := range []string{"ffmpeg", "fps=8", "scale=-2:720", "-frames:v 24", "/tmp/frames/frame-%03d.jpg"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command missing %q: %s", want, joined)
		}
	}
}
