package browser_test

import (
	"context"
	"errors"
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
	svc := newTestService(t, browser.Policy{})
	sess, _ := svc.Start(ctx, browser.StartRequest{SessionName: "lesson"})
	if err := svc.RecordLessonEvent(ctx, browser.LessonEvent{SessionID: sess.ID, Kind: "completion"}); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if svc.HasSession() {
		t.Fatalf("completion should close session")
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
