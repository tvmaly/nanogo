package cron_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/scheduler/cron"
)

func TestCronValidSpecs(t *testing.T) {
	t.Parallel()
	specs := []string{
		"0 9 * * 1-5",
		"*/30 * * * *",
		"0 0 1 * *",
	}
	for _, spec := range specs {
		s, err := cron.New(spec)
		if err != nil {
			t.Errorf("spec %q should be valid, got: %v", spec, err)
			continue
		}
		s.Stop()
	}
}

func TestCronInvalidSpec(t *testing.T) {
	t.Parallel()
	_, err := cron.New("not-a-cron-spec")
	if err == nil {
		t.Fatal("expected error for invalid spec")
	}
}

func TestCronFires(t *testing.T) {
	t.Parallel()
	var fired atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Every second so the test is fast.
	s, err := cron.New("* * * * * *") // 6-field: every second
	if err != nil {
		t.Fatal(err)
	}
	s.Start(ctx, func() { fired.Add(1) })
	defer s.Stop()

	// Wait for at least one firing.
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("cron never fired; got %d firings", fired.Load())
		case <-time.After(100 * time.Millisecond):
			if fired.Load() >= 1 {
				return
			}
		}
	}
}

func TestCronStops(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var fired atomic.Int32

	s, err := cron.New("* * * * * *")
	if err != nil {
		t.Fatal(err)
	}
	s.Start(ctx, func() { fired.Add(1) })
	time.Sleep(1500 * time.Millisecond)
	s.Stop()
	after := fired.Load()
	time.Sleep(1500 * time.Millisecond)
	if fired.Load() != after {
		t.Errorf("cron continued firing after Stop()")
	}
}
