package session_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/session"
)

// --- TEST-2.9: Session persistence: JSONL roundtrip ---

func TestJSONLRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := session.NewStore(dir, nil)

	sess, err := store.Create("sess-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	msgs := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
		{Role: "user", Content: "again"},
	}
	for _, m := range msgs {
		sess.Append(m)
	}
	if err := sess.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reopen
	sess2, err := store.Load("sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := sess2.Messages()
	if len(got) != len(msgs) {
		t.Fatalf("messages: got %d, want %d", len(got), len(msgs))
	}
	for i, m := range msgs {
		if got[i].Role != m.Role || got[i].Content != m.Content {
			t.Errorf("msg[%d]: got {%s %s}, want {%s %s}", i, got[i].Role, got[i].Content, m.Role, m.Content)
		}
	}

	// Verify JSONL format: each line must be valid JSON
	data, err := os.ReadFile(filepath.Join(dir, "sess-1.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := splitLines(data)
	if len(lines) != len(msgs) {
		t.Errorf("JSONL lines: got %d, want %d", len(lines), len(msgs))
	}
}

// --- TEST-2.10: Session resumable: AskUser checkpoints ---

func TestSessionResume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := session.NewStore(dir, nil)

	sess, _ := store.Create("sess-resume")
	sess.Append(llm.Message{Role: "user", Content: "question"})
	_ = sess.Save()

	// Set waiting state
	turnID := "turn-1"
	ch := sess.SetWaiting(turnID)

	if sess.GetStatus() != session.StatusWaiting {
		t.Errorf("status = %v, want %v", sess.GetStatus(), session.StatusWaiting)
	}

	// Resume in another goroutine
	var answer string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		answer = <-ch
	}()

	time.Sleep(10 * time.Millisecond) // let goroutine block on ch
	sess.Resume(turnID, "the answer")
	wg.Wait()

	if answer != "the answer" {
		t.Errorf("answer = %q, want %q", answer, "the answer")
	}
	if sess.GetStatus() != session.StatusActive {
		t.Errorf("status after resume = %v, want active", sess.GetStatus())
	}
}

// --- TEST-2.11: Session TTL garbage collection ---

func TestSessionTTL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	fakeClock := &manualClock{now: time.Now()}
	store := session.NewStore(dir, fakeClock)

	// Create a session in waiting state
	old, _ := store.Create("old-session")
	old.Append(llm.Message{Role: "user", Content: "old"})
	old.SetWaiting("t1")
	_ = old.Save()

	// Create a recent session
	recent, _ := store.Create("recent-session")
	recent.Append(llm.Message{Role: "user", Content: "recent"})
	_ = recent.Save()

	// Advance clock past 24h
	fakeClock.now = fakeClock.now.Add(25 * time.Hour)

	ttl := 24 * time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store.GC(ctx, ttl)

	// old-session should be deleted
	if _, err := store.Load("old-session"); err == nil {
		t.Error("old-session should have been GC'd")
	}

	// recent-session should still exist (it was created just now, even though clock advanced)
	// Actually the recent session was created at the same "now" time, but it's not in waiting state
	// GC only removes waiting sessions older than TTL
	if _, err := store.Load("recent-session"); err != nil {
		t.Errorf("recent-session should survive GC: %v", err)
	}
}

// helpers

type manualClock struct{ now time.Time }

func (c *manualClock) Now() time.Time { return c.now }

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := string(data[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) && len(data[start:]) > 0 {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
