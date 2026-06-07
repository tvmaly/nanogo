package browser_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/browser/fake"
)

func TestServiceStartReuseMaxAndTTL(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	bus := event.NewBus()
	svc, err := browser.NewService(browser.ServiceConfig{
		Controller: fake.New(),
		Bus:        bus,
		Now:        func() time.Time { return now },
		Policy:     browser.Policy{MaxSessions: 2, SessionTTLSeconds: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	s1, err := svc.Start(ctx, browser.StartRequest{SessionName: "lesson-1", Headed: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	again, err := svc.Start(ctx, browser.StartRequest{SessionName: "lesson-1", Headed: true})
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if again.ID != s1.ID {
		t.Fatalf("expected named session reuse, got %q then %q", s1.ID, again.ID)
	}
	if _, err := svc.Start(ctx, browser.StartRequest{SessionName: "lesson-2"}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if _, err := svc.Start(ctx, browser.StartRequest{SessionName: "lesson-3"}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("third code = %v err=%v", code(err), err)
	}
	now = now.Add(11 * time.Second)
	if err := svc.Cleanup(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if svc.HasSession() {
		t.Fatalf("expected ttl cleanup to close sessions")
	}
}

func TestServiceRejectsInvalidSessionName(t *testing.T) {
	svc := newTestService(t, browser.Policy{})
	if _, err := svc.Start(context.Background(), browser.StartRequest{SessionName: "../../escape"}); code(err) != browser.CodeInvalidRequest {
		t.Fatalf("code = %v err=%v", code(err), err)
	}
}

func TestServiceNavigationPolicyAndEvents(t *testing.T) {
	ctx := context.Background()
	bus := event.NewBus()
	sub := bus.Subscribe(ctx, event.Kind(browser.EventPageNavigated), event.Kind(browser.EventPageLoaded))
	svc, err := browser.NewService(browser.ServiceConfig{
		Controller: fake.New(),
		Bus:        bus,
		Policy:     browser.Policy{AllowedDomains: []string{"lessons.example.test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Start(ctx, browser.StartRequest{SessionName: "nav"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "https://blocked.example.test/lesson"}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("blocked code = %v err=%v", code(err), err)
	}
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "https://lessons.example.test/lesson", WaitUntil: "load"}); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	first := <-sub
	second := <-sub
	if string(first.Kind) != browser.EventPageNavigated || string(second.Kind) != browser.EventPageLoaded {
		t.Fatalf("event order = %s then %s", first.Kind, second.Kind)
	}
}

func TestServiceAllowsOnlyConfiguredFileRoots(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := newTestService(t, browser.Policy{AllowFileRoots: []string{root}})
	sess, err := svc.Start(ctx, browser.StartRequest{SessionName: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "file://" + root + "/index.html"}); err != nil {
		t.Fatalf("allowed file: %v", err)
	}
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "file:///private/index.html"}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("outside root code = %v err=%v", code(err), err)
	}
}

func TestServiceSnapshotRefsBecomeStaleAfterNavigation(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t, browser.Policy{})
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "refs"})
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "https://example.test/one"}); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.Snapshot(ctx, browser.SnapshotRequest{SessionID: sess.ID, InteractiveOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "https://example.test/two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Act(ctx, browser.ActionRequest{SessionID: sess.ID, Kind: browser.ActionClick, Target: browser.Target{Ref: snap.Nodes[0].Ref}}); code(err) != browser.CodeStaleRef {
		t.Fatalf("stale code = %v err=%v", code(err), err)
	}
}

func TestServiceEvalAndMediaPolicy(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t, browser.Policy{})
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "media"})
	if _, err := svc.Eval(ctx, browser.EvalRequest{SessionID: sess.ID, Script: "1"}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("eval denied code = %v err=%v", code(err), err)
	}
	if _, err := svc.MediaSeek(ctx, browser.MediaSeekRequest{SessionID: sess.ID, Seconds: 4, Strategy: "bogus"}); code(err) != browser.CodeUnsupportedStrategy {
		t.Fatalf("strategy code = %v err=%v", code(err), err)
	}
	res, err := svc.MediaSeek(ctx, browser.MediaSeekRequest{SessionID: sess.ID, Seconds: 12, Strategy: "auto"})
	if err != nil {
		t.Fatalf("media seek: %v", err)
	}
	if !res.Verified || res.StrategyUsed != "html5_video" {
		t.Fatalf("unexpected media seek: %+v", res)
	}
}

func TestLessonCompletionEventClosesSession(t *testing.T) {
	ctx := context.Background()
	bus := event.NewBus()
	sub := bus.Subscribe(ctx, event.Kind(browser.EventLessonCompleted))
	svc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New(), Policy: browser.Policy{}, Bus: bus})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "lesson"})
	if sess.LessonEventNonce == "" {
		t.Fatalf("start should return a lesson event nonce")
	}
	if err := svc.RecordLessonEvent(ctx, browser.LessonEvent{SessionID: sess.ID, Kind: "completion", Nonce: "wrong"}); code(err) != browser.CodeNotAuthorized {
		t.Fatalf("wrong nonce code = %v err=%v", code(err), err)
	}
	if svc.HasSession() == false {
		t.Fatalf("wrong nonce should not close session")
	}
	if err := svc.RecordLessonEvent(ctx, browser.LessonEvent{SessionID: sess.ID, Kind: "completion", Nonce: sess.LessonEventNonce}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	got := <-sub
	if string(got.Kind) != browser.EventLessonCompleted {
		t.Fatalf("event kind = %s", got.Kind)
	}
	if svc.HasSession() {
		t.Fatalf("completion should close session")
	}
}

func TestServiceRejectsStaleSessionWithStartGuidance(t *testing.T) {
	svc := newTestService(t, browser.Policy{})
	_, err := svc.Navigate(context.Background(), browser.NavigateRequest{SessionID: "closed", URL: "https://example.test"})
	if code(err) != browser.CodeNotFound || !strings.Contains(err.Error(), "browser_session_start") {
		t.Fatalf("stale session err = %v code=%v", err, code(err))
	}
}

func TestServiceConcurrentStartsReserveMaxSessions(t *testing.T) {
	ctx := context.Background()
	ctrl := fake.New()
	ctrl.StartDelay = 20 * time.Millisecond
	svc, err := browser.NewService(browser.ServiceConfig{Controller: ctrl, Policy: browser.Policy{MaxSessions: 1}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"one", "two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, err := svc.Start(ctx, browser.StartRequest{SessionName: name})
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)
	successes := 0
	policyDenied := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if code(err) == browser.CodePolicyDenied {
			policyDenied++
		}
	}
	if successes != 1 || policyDenied != 1 {
		t.Fatalf("successes=%d policyDenied=%d", successes, policyDenied)
	}
}

func TestServiceRegistryWritesSafeVersionedShape(t *testing.T) {
	ctx := context.Background()
	registry := filepath.Join(t.TempDir(), ".nanogo", "browser-sessions.json")
	svc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New(), Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(ctx, browser.StartRequest{SessionName: "safe", Headed: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "profile") || strings.Contains(string(data), "screenshot") || strings.Contains(string(data), "cookie") {
		t.Fatalf("registry contains sensitive field: %s", data)
	}
	var doc struct {
		Version  int `json:"version"`
		Sessions []struct {
			SessionID  string `json:"session_id"`
			Driver     string `json:"driver"`
			TTLSeconds int    `json:"ttl_seconds"`
			Headed     bool   `json:"headed"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || len(doc.Sessions) != 1 || doc.Sessions[0].SessionID == "" || doc.Sessions[0].TTLSeconds <= 0 || !doc.Sessions[0].Headed {
		t.Fatalf("bad registry shape: %+v", doc)
	}
}

func TestServiceRejectsSymlinkFileRootEscape(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.html"), []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	svc := newTestService(t, browser.Policy{AllowFileRoots: []string{root}})
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "files"})
	escaped := "file://" + filepath.ToSlash(filepath.Join(link, "secret.html"))
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: escaped}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("symlink escape code = %v err=%v", code(err), err)
	}
}

func TestServiceRejectsExternalScriptsInStrictLocalWrapper(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	wrapper := filepath.Join(root, "index.html")
	if err := os.WriteFile(wrapper, []byte(`<html><script src="https://cdn.example.test/app.js"></script></html>`), 0644); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t, browser.Policy{AllowFileRoots: []string{root}})
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "strict"})
	if _, err := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "file://" + filepath.ToSlash(wrapper)}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("external script code = %v err=%v", code(err), err)
	}
}

func TestServiceCanonicalizesArtifactPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	ctrl := fake.New()
	svc, err := browser.NewService(browser.ServiceConfig{Controller: ctrl, Policy: browser.Policy{ArtifactRoot: root}})
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "shot"})
	artifact, err := svc.Screenshot(ctx, browser.ScreenshotRequest{SessionID: sess.ID, Path: "nested/shot.png"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(artifact.Path) {
		t.Fatalf("artifact path is not absolute: %q", artifact.Path)
	}
	rel, err := filepath.Rel(root, artifact.Path)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("artifact path outside root: %q rel=%q err=%v", artifact.Path, rel, err)
	}
	if ctrl.LastScreenshotPath != artifact.Path {
		t.Fatalf("controller saw %q want %q", ctrl.LastScreenshotPath, artifact.Path)
	}
	if _, err := svc.Screenshot(ctx, browser.ScreenshotRequest{SessionID: sess.ID, Path: "../escape.png"}); code(err) != browser.CodePolicyDenied {
		t.Fatalf("escape code = %v err=%v", code(err), err)
	}
}

func newTestService(t *testing.T, p browser.Policy) *browser.Service {
	t.Helper()
	svc, err := browser.NewService(browser.ServiceConfig{Controller: fake.New(), Policy: p, Bus: event.NewBus()})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func code(err error) browser.ErrorCode {
	var be *browser.Error
	if errors.As(err, &be) {
		return be.Code
	}
	return ""
}
