package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/transport/webui"
)

func TestWebUITransport_HealthAndShells(t *testing.T) {
	t.Parallel()
	srv := webui.New(webui.Config{})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	for _, path := range []string{"/healthz", "/student", "/parent"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d", path, resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if path != "/healthz" && !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s: want text/html, got %q", path, ct)
		}
	}
}

func TestStudentLessonRender(t *testing.T) {
	t.Parallel()
	lesson := webui.Lesson{
		ID:    "lesson-1",
		Title: "Fractions Intro",
		Blocks: []webui.Block{
			{ID: "b1", Kind: "prose", Content: "Learn about fractions."},
			{ID: "b2", Kind: "video", VideoURL: "https://www.youtube.com/embed/abc123"},
			{ID: "b3", Kind: "quiz", QuizRef: "quiz-fractions"},
			{ID: "b4", Kind: "interactive", AssetPath: "/lessons/fractions/game.js"},
			{ID: "b5", Kind: "manim", AssetPath: "/lessons/fractions/scene.mp4", Caption: "Number line"},
		},
	}
	srv := webui.New(webui.Config{Lessons: []webui.Lesson{lesson}, InsecureSkipAuth: true})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/student/lesson/lesson-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var bodyBytes strings.Builder
	io.Copy(&bodyBytes, resp.Body)
	body := bodyBytes.String()

	for _, want := range []string{"b1", "b2", "b3", "b4", "b5", "Fractions Intro"} {
		if !strings.Contains(body, want) {
			t.Errorf("lesson render: body missing %q", want)
		}
	}
}

func TestVideoEmbedAllowlist(t *testing.T) {
	t.Parallel()
	allowed := []string{
		"https://www.youtube.com/embed/abc",
		"https://youtu.be/abc",
		"https://vimeo.com/123",
		"https://player.vimeo.com/video/123",
	}
	for _, u := range allowed {
		if err := webui.ValidateVideoURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}

	blocked := []string{
		"https://evil.example/video.mp4",
		"https://cdn.attacker.io/embed",
	}
	for _, u := range blocked {
		if err := webui.ValidateVideoURL(u); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}
}

func TestInteractiveAssetPolicy(t *testing.T) {
	t.Parallel()
	// same-origin paths are OK
	if err := webui.ValidateInteractiveAsset("/lessons/fractions/game.js"); err != nil {
		t.Errorf("expected same-origin path to be allowed: %v", err)
	}
	if err := webui.ValidateInteractiveAsset("/lessons/fractions/game.ts"); err != nil {
		t.Errorf("expected same-origin .ts path to be allowed: %v", err)
	}
	// remote URLs are rejected
	if err := webui.ValidateInteractiveAsset("https://cdn.example/app.js"); err == nil {
		t.Error("expected remote URL to be rejected")
	}
}

func TestRoleBoundaries(t *testing.T) {
	t.Parallel()
	srv := webui.New(webui.Config{})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Parent-only routes should return 403 without a parent session cookie.
	parentOnlyPaths := []string{
		"/parent/reports",
		"/parent/lessons/new",
		"/parent/dashboard",
	}
	for _, path := range parentOnlyPaths {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s: want 403, got %d", path, resp.StatusCode)
		}
	}

	// Student routes should return 403 without a student session cookie.
	studentOnlyPaths := []string{"/student/lesson/lesson-1"}
	for _, path := range studentOnlyPaths {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s: want 403, got %d", path, resp.StatusCode)
		}
	}
}

func TestManimLessonBlocks(t *testing.T) {
	t.Parallel()
	lesson := webui.Lesson{
		ID:    "manim-lesson",
		Title: "Physics Demo",
		Blocks: []webui.Block{
			{ID: "m1", Kind: "manim", AssetPath: "/lessons/physics/scene.mp4", Caption: "Pendulum"},
		},
	}
	srv := webui.New(webui.Config{
		Lessons: []webui.Lesson{lesson},
		// bypass auth for test
		InsecureSkipAuth: true,
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/student/lesson/manim-lesson")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var bodyBuf strings.Builder
	io.Copy(&bodyBuf, resp.Body)
	body := bodyBuf.String()
	if !strings.Contains(body, "scene.mp4") {
		t.Error("manim block: expected asset path in rendered page")
	}
	if !strings.Contains(body, "Pendulum") {
		t.Error("manim block: expected caption in rendered page")
	}
}
