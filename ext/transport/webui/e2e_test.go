package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/transport/webui"
)

// TestE2EHomeschoolWorkflow covers TEST-11.11: an end-to-end homeschool
// session — server boots, student and parent shells load, a lesson renders,
// and role boundaries are enforced.
func TestE2EHomeschoolWorkflow(t *testing.T) {
	lesson := webui.Lesson{
		ID:    "math-fractions",
		Title: "Intro to Fractions",
		Blocks: []webui.Block{
			{ID: "b1", Kind: "prose", Content: "Fractions represent parts of a whole."},
			{ID: "b2", Kind: "video", VideoURL: "https://www.youtube.com/embed/n0tFNi4hOm4"},
			{ID: "b3", Kind: "quiz", QuizRef: "quiz-fractions-1"},
			{ID: "b4", Kind: "interactive", AssetPath: "/lessons/fractions/sim.js"},
			{ID: "b5", Kind: "manim", AssetPath: "/lessons/fractions/scene.mp4", Caption: "Number line"},
		},
	}

	// Server with auth enabled (default).
	srv := webui.New(webui.Config{Lessons: []webui.Lesson{lesson}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := ts.Client()

	t.Run("healthz", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("student_shell_public", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/student")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("student shell: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("parent_shell_public", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/parent")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("parent shell: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("lesson_requires_student_role", func(t *testing.T) {
		// No cookie → 403.
		resp, err := client.Get(ts.URL + "/student/lesson/math-fractions")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("want 403 without role cookie, got %d", resp.StatusCode)
		}
	})

	t.Run("lesson_renders_with_student_role", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/student/lesson/math-fractions", nil)
		req.AddCookie(&http.Cookie{Name: "role", Value: "student"})
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200 with student cookie, got %d", resp.StatusCode)
		}
		var body strings.Builder
		io.Copy(&body, resp.Body)
		for _, want := range []string{"Intro to Fractions", "b1", "b2", "b3", "b4", "b5", "scene.mp4"} {
			if !strings.Contains(body.String(), want) {
				t.Errorf("lesson body missing %q", want)
			}
		}
	})

	t.Run("parent_dashboard_requires_parent_role", func(t *testing.T) {
		// No cookie → 403.
		resp, err := client.Get(ts.URL + "/parent/dashboard")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("want 403 without role cookie, got %d", resp.StatusCode)
		}
	})

	t.Run("parent_dashboard_accessible_with_parent_role", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/parent/dashboard", nil)
		req.AddCookie(&http.Cookie{Name: "role", Value: "parent"})
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("parent dashboard: want 200 with parent cookie, got %d", resp.StatusCode)
		}
	})

	t.Run("student_cannot_reach_parent_pages", func(t *testing.T) {
		for _, path := range []string{"/parent/dashboard", "/parent/reports", "/parent/lessons/new"} {
			req, _ := http.NewRequest("GET", ts.URL+path, nil)
			req.AddCookie(&http.Cookie{Name: "role", Value: "student"})
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("GET %s with student cookie: want 403, got %d", path, resp.StatusCode)
			}
		}
	})
}
