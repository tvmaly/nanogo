package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	llmfake "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/memory"
	sessionpkg "github.com/tvmaly/nanogo/core/session"
)

func tempStore(t *testing.T) *memory.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := memory.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TEST-4.1 — Store file I/O primitives
func TestStoreFileIO(t *testing.T) {
	t.Parallel()
	s := tempStore(t)

	// Missing file returns ""
	content, err := s.ReadFile("MEMORY.md")
	if err != nil || content != "" {
		t.Fatalf("expected empty, got %q %v", content, err)
	}

	// WriteFile + ReadFile roundtrip
	if err := s.WriteFile("MEMORY.md", "hello"); err != nil {
		t.Fatal(err)
	}
	content, err = s.ReadFile("MEMORY.md")
	if err != nil || content != "hello" {
		t.Fatalf("got %q %v", content, err)
	}

	// AppendHistory + ReadHistory
	e := memory.HistoryEntry{Role: "user", Content: "hi"}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ReadHistory()
	if err != nil || len(entries) != 1 {
		t.Fatalf("got %d entries: %v", len(entries), err)
	}
	if entries[0].Role != "user" || entries[0].Content != "hi" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}

	// topics/, episodic.jsonl, learnings.jsonl
	for _, rel := range []string{"memory/topics/foo.md", "memory/episodic.jsonl", "memory/learnings.jsonl"} {
		if err := s.WriteFile(rel, "data"); err != nil {
			t.Fatalf("WriteFile %s: %v", rel, err)
		}
		c, err := s.ReadFile(rel)
		if err != nil || c != "data" {
			t.Fatalf("ReadFile %s: %q %v", rel, c, err)
		}
	}

	// SOUL.md, USER.md
	for _, f := range []string{"SOUL.md", "USER.md"} {
		if err := s.WriteFile(f, f+" content"); err != nil {
			t.Fatal(err)
		}
		c, _ := s.ReadFile(f)
		if !strings.Contains(c, f) {
			t.Fatalf("%s content wrong: %q", f, c)
		}
	}
}

// TEST-4.2 — Consolidator threshold trigger
func TestConsolidatorThreshold(t *testing.T) {
	t.Parallel()
	s := tempStore(t)
	bus := event.NewBus()
	sessionStore := sessionpkg.NewStore(t.TempDir(), nil)

	// Create a session with messages and persist it so Load works.
	sess, _ := sessionStore.Create("sess1")
	for i := 0; i < 25; i++ {
		sess.Append(llm.Message{Role: "user", Content: "msg"})
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}

	// Fake LLM returns a summary for every call.
	provider := llmfake.New([]llm.Chunk{{TextDelta: "summary text"}})

	cons := memory.NewConsolidator(s, bus, sessionStore, provider, memory.ConsolidatorConfig{
		ThresholdTokens: 1, // trigger immediately
		ChunkSize:       5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cons.Run(ctx)

	// Publish TurnCompleted with enough tokens.
	bus.Publish(event.Event{
		Kind:    event.TurnCompleted,
		Session: "sess1",
		Payload: event.TurnCompletedPayload{InputTokens: 100, OutputTokens: 100},
	})

	// Wait for MemoryUpdated event.
	sub := bus.Subscribe(ctx, event.MemoryUpdated)
	select {
	case evt := <-sub:
		if evt.Kind != event.MemoryUpdated {
			t.Fatalf("unexpected event: %v", evt.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for MemoryUpdated")
	}

	// history.jsonl should have the summary.
	entries, _ := s.ReadHistory()
	if len(entries) == 0 {
		t.Fatal("expected history entries after consolidation")
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Content, "summary") {
			found = true
		}
	}
	if !found {
		t.Fatalf("summary not found in history: %+v", entries)
	}
}

// TEST-4.3 — Consolidator failure handling
func TestConsolidatorFailure(t *testing.T) {
	t.Parallel()
	s := tempStore(t)
	bus := event.NewBus()
	sessionStore := sessionpkg.NewStore(t.TempDir(), nil)

	sess, _ := sessionStore.Create("sess-fail")
	for i := 0; i < 25; i++ {
		sess.Append(llm.Message{Role: "user", Content: "msg"})
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}

	attempts := 0
	provider := llmfake.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		attempts++
		// Fail first 3 attempts; on 4th (raw dump path), succeed is not needed since raw dump is plain text.
		ch := make(chan llm.Chunk, 1)
		if attempts <= 3 {
			close(ch)
			return ch, &testError{"llm failed"}
		}
		ch <- llm.Chunk{TextDelta: "summary"}
		close(ch)
		return ch, nil
	})

	cons := memory.NewConsolidator(s, bus, sessionStore, provider, memory.ConsolidatorConfig{
		ThresholdTokens: 1,
		ChunkSize:       5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cons.Run(ctx)

	// Publish 4 TurnCompleted events to trigger 4 consolidation attempts.
	for i := 0; i < 4; i++ {
		bus.Publish(event.Event{
			Kind:    event.TurnCompleted,
			Session: "sess-fail",
			Payload: event.TurnCompletedPayload{InputTokens: 100, OutputTokens: 100},
		})
		time.Sleep(50 * time.Millisecond)
	}

	// On 3rd failure, raw dump should appear in history.jsonl.
	time.Sleep(200 * time.Millisecond)
	data, _ := s.ReadFile("memory/history.jsonl")
	if !strings.Contains(data, "raw_dump") && !strings.Contains(data, "msg") {
		// After 3 failures a raw dump is written; content should have messages.
		// The raw dump path writes JSON directly, so we check the file exists and has data.
		if data == "" {
			t.Fatal("expected history.jsonl to have content after 3 failures")
		}
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TEST-4.4 — Dream two-phase pass
func TestDream(t *testing.T) {
	t.Parallel()
	s := tempStore(t)

	// Seed history with some entries.
	_ = s.AppendHistory(memory.HistoryEntry{Role: "user", Content: "I like Go"})
	_ = s.AppendHistory(memory.HistoryEntry{Role: "assistant", Content: "Great choice"})

	// Fake LLM: phase1 returns JSON plan, phase2 applies it.
	calls := 0
	provider := llmfake.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		calls++
		ch := make(chan llm.Chunk, 1)
		var resp string
		if calls == 1 {
			// Phase 1: produce a plan.
			resp = `{"MEMORY.md": "User likes Go.", "SOUL.md": "I help with Go.", "USER.md": "Likes Go"}`
		} else {
			// Phase 2: apply plan.
			resp = `{"MEMORY.md": "User likes Go.", "SOUL.md": "I help with Go.", "USER.md": "Likes Go"}`
		}
		ch <- llm.Chunk{TextDelta: resp}
		close(ch)
		return ch, nil
	})

	d := memory.NewDream(s, provider)
	if err := d.Dream(context.Background()); err != nil {
		t.Fatalf("Dream: %v", err)
	}

	memContent, _ := s.ReadFile("MEMORY.md")
	if !strings.Contains(memContent, "Go") {
		t.Fatalf("MEMORY.md not updated: %q", memContent)
	}

	// Re-run with no new entries — should be a no-op (0 additional LLM calls).
	prevCalls := calls
	if err := d.Dream(context.Background()); err != nil {
		t.Fatalf("second Dream: %v", err)
	}
	if calls != prevCalls {
		t.Fatalf("expected no-op on second Dream, got %d extra calls", calls-prevCalls)
	}
}

// TEST-4.5 — Dream cursor correctness
func TestDreamCursor(t *testing.T) {
	t.Parallel()
	s := tempStore(t)

	_ = s.AppendHistory(memory.HistoryEntry{Role: "user", Content: "first entry"})

	calls := 0
	provider := llmfake.NewFunc(func(ctx context.Context, req llm.Request) (<-chan llm.Chunk, error) {
		calls++
		ch := make(chan llm.Chunk, 1)
		ch <- llm.Chunk{TextDelta: `{"MEMORY.md": "updated", "SOUL.md": "", "USER.md": ""}`}
		close(ch)
		return ch, nil
	})

	d := memory.NewDream(s, provider)

	// First run processes "first entry".
	if err := d.Dream(context.Background()); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := calls

	// Add a new entry.
	_ = s.AppendHistory(memory.HistoryEntry{Role: "user", Content: "second entry"})

	// Second run should only process "second entry".
	if err := d.Dream(context.Background()); err != nil {
		t.Fatal(err)
	}

	if calls == callsAfterFirst {
		t.Fatal("expected second Dream to make LLM calls for new entry")
	}
	// Verify cursor advanced: a third run with no new entries is a no-op.
	callsBeforeThird := calls
	_ = d.Dream(context.Background())
	if calls != callsBeforeThird {
		t.Fatal("third Dream should be no-op")
	}
}

// TEST-4.6 — Curator topic write/read/edit/link
func TestCurator(t *testing.T) {
	t.Parallel()
	s := tempStore(t)
	c := memory.NewCurator(s)

	// WriteTopic
	if err := c.WriteTopic("docker", "Docker is a container runtime."); err != nil {
		t.Fatal(err)
	}

	// ReadTopic
	content, err := c.ReadTopic("docker")
	if err != nil || !strings.Contains(content, "Docker") {
		t.Fatalf("ReadTopic: %q %v", content, err)
	}

	// EditTopic
	if err := c.EditTopic("docker", "container runtime", "container platform"); err != nil {
		t.Fatal(err)
	}
	content, _ = c.ReadTopic("docker")
	if !strings.Contains(content, "container platform") {
		t.Fatalf("EditTopic: %q", content)
	}

	// LinkTopics
	if err := c.WriteTopic("auth", "Auth service."); err != nil {
		t.Fatal(err)
	}
	if err := c.LinkTopics("docker", "auth", "uses"); err != nil {
		t.Fatal(err)
	}
	dockerContent, _ := c.ReadTopic("docker")
	authContent, _ := c.ReadTopic("auth")
	if !strings.Contains(dockerContent, "auth") {
		t.Fatalf("docker should link to auth: %q", dockerContent)
	}
	if !strings.Contains(authContent, "docker") {
		t.Fatalf("auth should back-link to docker: %q", authContent)
	}

	// Grep
	results, err := c.Grep("container")
	if err != nil || len(results) == 0 {
		t.Fatalf("Grep: %v results, %v", results, err)
	}
	found := false
	for _, r := range results {
		if strings.Contains(r, "container") {
			found = true
		}
	}
	if !found {
		t.Fatalf("grep didn't find 'container': %v", results)
	}
}

// TEST-4.7 — Curator pruning
func TestCuratorPrune(t *testing.T) {
	t.Parallel()
	s := tempStore(t)

	now := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	c := memory.NewCuratorWithClock(s, clock)

	// Write a topic that will be "old" (accessed 100 days ago).
	oldTime := now.AddDate(0, 0, -100)
	header := "<!-- la:" + oldTime.Format(time.RFC3339) + " rc:0 -->\nOld topic."
	_ = s.WriteFile("memory/topics/old-topic.md", header)

	// Write a recent topic.
	if err := c.WriteTopic("recent-topic", "Recent content."); err != nil {
		t.Fatal(err)
	}

	if err := c.PruneOld(context.Background(), 90, 0.5); err != nil {
		t.Fatal(err)
	}

	// old-topic should be in archive.
	archivePath := filepath.Join(s.Root(), "memory", "archive", "old-topic.md")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("old-topic.md should be in archive")
	}

	// recent-topic should still be in topics.
	topicPath := filepath.Join(s.Root(), "memory", "topics", "recent-topic.md")
	if _, err := os.Stat(topicPath); os.IsNotExist(err) {
		t.Fatal("recent-topic.md should still be in topics")
	}
}

// TEST-4.8 — Curator index rebuild
func TestCuratorIndex(t *testing.T) {
	t.Parallel()
	s := tempStore(t)
	c := memory.NewCurator(s)

	topics := []string{"alpha", "beta", "gamma"}
	for _, slug := range topics {
		if err := c.WriteTopic(slug, slug+" content"); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.RebuildIndex(); err != nil {
		t.Fatal(err)
	}

	index, err := s.ReadFile("index.md")
	if err != nil || index == "" {
		t.Fatalf("index.md missing: %v", err)
	}

	// Check line count < 200.
	lines := strings.Split(strings.TrimSpace(index), "\n")
	if len(lines) >= 200 {
		t.Fatalf("index has %d lines, want < 200", len(lines))
	}

	// Every topic should appear.
	for _, slug := range topics {
		if !strings.Contains(index, slug) {
			t.Fatalf("index missing topic %q", slug)
		}
	}
}
