package scheduler_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/scheduler"
	fakesched "github.com/tvmaly/nanogo/core/scheduler/fake"
)

func TestRegistry(t *testing.T) {
	t.Parallel()
	called := false
	scheduler.Register("test-sched", func(_ json.RawMessage) (scheduler.Scheduler, error) {
		called = true
		return fakesched.New(), nil
	})
	s, err := scheduler.Build("test-sched", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("factory not called")
	}
	if s == nil {
		t.Fatal("nil scheduler returned")
	}
}

func TestRegistryUnknown(t *testing.T) {
	t.Parallel()
	_, err := scheduler.Build("no-such-driver", nil)
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestFakeScheduler(t *testing.T) {
	t.Parallel()
	s := fakesched.New()

	fired := 0
	if err := s.Schedule("j1", "every 5m", func(_ context.Context) { fired++ }); err != nil {
		t.Fatal(err)
	}

	jobs := s.List()
	if len(jobs) != 1 || jobs[0].ID != "j1" {
		t.Fatalf("expected 1 job, got %v", jobs)
	}

	ok := s.Fire(context.Background(), "j1")
	if !ok || fired != 1 {
		t.Fatalf("Fire failed or fn not called: ok=%v fired=%d", ok, fired)
	}

	if err := s.Remove("j1"); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Fatal("job still present after Remove")
	}
}
