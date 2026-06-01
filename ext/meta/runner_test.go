package meta

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFakeManimSuccessWritesArtifactsEvidenceAndGraph(t *testing.T) {
	store := &FakeStore{}
	svc := testService(t.TempDir(), store)
	res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindManimLesson, Prompt: "Teach multiplying fractions to a 9-year-old", Runner: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != DecisionAccepted || !res.Eligible {
		t.Fatalf("decision = %q eligible=%v", res.Decision, res.Eligible)
	}
	assertExists(t, res.BundlePath)
	assertExists(t, res.VideoPath)
	assertExists(t, filepath.Join(res.RunDir, "render.stdout.log"))
	assertExists(t, filepath.Join(res.RunDir, "render.stderr.log"))
	assertExists(t, res.ValidationPath)
	assertExists(t, res.PreviewPath)
	assertLineageEvent(t, store, "created")
	assertLineageEvent(t, store, "tested")
	assertLineageEvent(t, store, "accepted")
	assertGraphRelation(t, store, "renders")
	assertGraphRelation(t, store, "validates")
}

func TestFakeManimRejectsMissingVideoAndLogs(t *testing.T) {
	store := &FakeStore{}
	svc := testService(t.TempDir(), store)
	svc.Fake.MissingVideo = true
	res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindManimLesson, Prompt: "Teach multiplying fractions", Runner: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != DecisionRejected || !containsReason(res.FailureReasons, "video") {
		t.Fatalf("missing video decision=%q reasons=%v", res.Decision, res.FailureReasons)
	}

	store = &FakeStore{}
	svc = testService(t.TempDir(), store)
	svc.Fake.MissingRenderLogs = true
	res, err = svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindManimLesson, Prompt: "Teach multiplying fractions", Runner: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != DecisionRejected || !containsReason(res.FailureReasons, "render logs") {
		t.Fatalf("missing logs decision=%q reasons=%v", res.Decision, res.FailureReasons)
	}
}

func TestFakeBrowserGameSuccessWritesArtifactsEvidenceAndGraph(t *testing.T) {
	store := &FakeStore{}
	svc := testService(t.TempDir(), store)
	res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindBrowserGameLesson, Prompt: "Teach states of matter with a drag-and-drop sorting game", Runner: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != DecisionAccepted || res.PreviewPath == "" || res.PreviewURL == "" {
		t.Fatalf("result = %+v", res)
	}
	for _, rel := range []string{"src/main.js", "dist/index.html", "screenshots/preview.png", "validation_report.json", "preview/index.html"} {
		assertExists(t, filepath.Join(res.RunDir, rel))
	}
	assertLineageEvent(t, store, "accepted")
	assertGraphRelation(t, store, "builds")
	assertGraphRelation(t, store, "previews")
	assertGraphRelation(t, store, "captures")
}

func TestFakeBrowserGameRejectsConsoleErrorsHappyPathAndBudget(t *testing.T) {
	t.Run("console_errors", func(t *testing.T) {
		store := &FakeStore{}
		svc := testService(t.TempDir(), store)
		svc.Fake.BrowserConsoleErrs = 1
		res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindBrowserGameLesson, Prompt: "Teach states of matter", Runner: "fake"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Decision != DecisionRejected || !containsReason(res.FailureReasons, "console errors") {
			t.Fatalf("console decision=%q reasons=%v", res.Decision, res.FailureReasons)
		}
		if len(store.Evidence) == 0 || store.Evidence[0].Kind != "console_error" {
			t.Fatalf("evidence = %+v", store.Evidence)
		}
		assertGraphRelation(t, store, "failed_because")
	})
	t.Run("happy_path", func(t *testing.T) {
		svc := testService(t.TempDir(), &FakeStore{})
		svc.Fake.HappyPathFails = true
		res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindBrowserGameLesson, Prompt: "Teach states of matter", Runner: "fake"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Decision != DecisionRejected || !containsReason(res.FailureReasons, "happy-path") {
			t.Fatalf("happy path decision=%q reasons=%v", res.Decision, res.FailureReasons)
		}
	})
	t.Run("budget", func(t *testing.T) {
		svc := testService(t.TempDir(), &FakeStore{})
		svc.Fake.BudgetExceeded = true
		res, err := svc.CreateLesson(context.Background(), CreateLessonRequest{Kind: KindBrowserGameLesson, Prompt: "Teach states of matter", Runner: "fake"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Decision != DecisionBudgetExceeded || !containsReason(res.FailureReasons, "budget") {
			t.Fatalf("budget decision=%q reasons=%v", res.Decision, res.FailureReasons)
		}
		assertExists(t, filepath.Join(res.RunDir, "build.stderr.log"))
	})
}

func testService(root string, store EvidenceStore) *Service {
	svc := NewService(root, store)
	svc.Clock = func() time.Time { return time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC) }
	return svc
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		t.Fatal("empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertLineageEvent(t *testing.T, store *FakeStore, event string) {
	t.Helper()
	for _, rec := range store.Lineage {
		if rec.Event == event {
			return
		}
	}
	t.Fatalf("missing lineage event %q in %+v", event, store.Lineage)
}

func assertGraphRelation(t *testing.T, store *FakeStore, rel string) {
	t.Helper()
	for _, edge := range store.Graph {
		if edge.Relation == rel {
			return
		}
	}
	t.Fatalf("missing graph relation %q in %+v", rel, store.Graph)
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}
