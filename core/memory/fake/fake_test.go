package fake_test

import (
	"context"
	"testing"
	"time"

	memfake "github.com/tvmaly/nanogo/core/memory/fake"
)

func TestFakes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &memfake.Consolidator{}
	done := make(chan struct{})
	go func() { _ = c.Run(ctx); close(done) }()
	time.Sleep(time.Millisecond)
	cancel()
	<-done
	if !c.Running {
		t.Fatal("consolidator did not run")
	}

	d := &memfake.Dreamer{}
	if err := d.Dream(context.Background()); err != nil || d.Calls != 1 {
		t.Fatalf("dream = %v calls=%d", err, d.Calls)
	}

	store := memfake.NewCuratorStore()
	_ = store.WriteTopic("go", "Go is fast\nGo has tests")
	_ = store.EditTopic("go", "fast", "small")
	got, _ := store.ReadTopic("go")
	if got != "Go is small\nGo has tests" {
		t.Fatalf("topic = %q", got)
	}
	_ = store.WriteTopic("tests", "testing")
	_ = store.LinkTopics("go", "tests", "relates")
	matches, _ := store.Grep("Go")
	if len(matches) != 2 {
		t.Fatalf("matches = %v", matches)
	}
	_ = store.PruneOld(context.Background(), 0, 0)
	_ = store.RebuildIndex()
	if store.Index == "" {
		t.Fatal("index not rebuilt")
	}
}
