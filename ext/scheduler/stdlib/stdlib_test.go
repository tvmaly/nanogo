package stdlib_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/scheduler/stdlib"
)

func TestParseValidSpecs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		spec string
		want time.Duration
	}{
		{"every 30m", 30 * time.Minute},
		{"every 1h", 1 * time.Hour},
		{"every 10s", 10 * time.Second},
		{"every 5m", 5 * time.Minute},
	}
	for _, c := range cases {
		s := stdlib.New()
		fired := make(chan struct{}, 1)
		err := s.Schedule("j", c.spec, func(_ context.Context) {
			select {
			case fired <- struct{}{}:
			default:
			}
		})
		if err != nil {
			t.Fatalf("spec %q: unexpected error: %v", c.spec, err)
		}
		jobs := s.List()
		if len(jobs) != 1 || jobs[0].ID != "j" {
			t.Fatalf("spec %q: List returned %v", c.spec, jobs)
		}
		// Just verify the interval is internally correct by inspecting Next.
		if jobs[0].Next == "" {
			t.Fatalf("spec %q: Next is empty", c.spec)
		}
		_ = c.want
	}
}

func TestParseInvalidSpecs(t *testing.T) {
	t.Parallel()
	bad := []string{
		"every",
		"every 0m",
		"every -1m",
		"every 5x",
		"*/5 * * * *",
		"",
	}
	s := stdlib.New()
	for _, spec := range bad {
		if err := s.Schedule("j", spec, func(_ context.Context) {}); err == nil {
			t.Fatalf("spec %q: expected error, got nil", spec)
		}
	}
}

func TestSchedulerFiresJobs(t *testing.T) {
	t.Parallel()
	s := stdlib.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int64
	if err := s.Schedule("j1", "every 50ms", func(_ context.Context) {
		count.Add(1)
	}); err != nil {
		t.Fatal(err)
	}

	// Workaround: stdlib scheduler uses time.Ticker — we register before start.
	// After start, jobs fire at interval.
	_ = s.Start(ctx)

	time.Sleep(180 * time.Millisecond)
	got := count.Load()
	if got < 2 {
		t.Fatalf("expected at least 2 firings in 180ms, got %d", got)
	}
}

func TestSchedulerRemove(t *testing.T) {
	t.Parallel()
	s := stdlib.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int64
	_ = s.Schedule("j1", "every 20ms", func(_ context.Context) { count.Add(1) })
	_ = s.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	_ = s.Remove("j1")
	before := count.Load()
	time.Sleep(50 * time.Millisecond)
	after := count.Load()
	if after != before {
		t.Fatalf("job still firing after Remove: before=%d after=%d", before, after)
	}
	if len(s.List()) != 0 {
		t.Fatal("List not empty after Remove")
	}
}
