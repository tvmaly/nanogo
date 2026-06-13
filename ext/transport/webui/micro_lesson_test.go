package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/transport/webui"
)

func TestMicroLessonPlayerUsesStrictScriptPolicyAndSegmentBounds(t *testing.T) {
	srv := webui.New(webui.Config{InsecureSkipAuth: true, MicroLessons: []webui.MicroLesson{{
		ID: "ml-01", Title: "The Basic Throw", ChildSafetySetup: "Stand away from breakables.", ParentSafetySetup: "Parent-only note",
		YouTubeVideoID: "abc123", StartSeconds: 42, EndSeconds: 130,
	}}})
	ts := httptest.NewServer(srv)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/student/micro-lesson/ml-01")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	body := string(data)
	for _, want := range []string{"The Basic Throw", "Stand away from breakables.", "start=42", "end=130", "Capture attempt"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "htmx") || strings.Contains(body, "unpkg.com") || strings.Contains(body, "Parent-only note") {
		t.Fatalf("strict player leaked script or parent text: %s", body)
	}
	if strings.Index(body, "Stand away") > strings.Index(body, "Capture attempt") {
		t.Fatalf("safety setup should render before capture: %s", body)
	}
}
