package evolve_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/evolve"
)

// TEST-9.1 — Sandbox: git worktree creation and cleanup
func TestSandbox(t *testing.T) {
	t.Parallel()
	// Use the repo root as the source repo for the worktree.
	repoRoot := findRepoRoot(t)
	sb, err := evolve.NewSandbox(repoRoot)
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	if _, err := os.Stat(sb.Dir()); err != nil {
		t.Fatalf("worktree dir not found: %v", err)
	}
	if err := sb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(sb.Dir()); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still exists after Close")
	}
}

// TEST-9.2 — Path guard: core/ and ext/evolve/ are blocked
func TestPathGuard(t *testing.T) {
	t.Parallel()
	blocked := []string{
		"core/agent/loop.go",
		"core/event/event.go",
		"ext/evolve/loop.go",
		"ext/evolve/sandbox.go",
	}
	for _, p := range blocked {
		if !evolve.IsBlocked(p) {
			t.Errorf("expected %q to be blocked", p)
		}
	}
	allowed := []string{
		"ext/llm/openai/client.go",
		"ext/obs/slog/slog.go",
		"pkg/nanogo/api.go",
	}
	for _, p := range allowed {
		if evolve.IsBlocked(p) {
			t.Errorf("expected %q to be allowed", p)
		}
	}
}

// TEST-9.2 — Learnings entry written on path-guard rejection
func TestPathGuardLearningsEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proposal := evolve.Proposal{
		Summary: "evil edit",
		Patches: []evolve.Patch{{Path: "core/agent/loop.go", Content: "bad"}},
	}
	err := evolve.ValidatePaths(proposal, memDir)
	if err == nil {
		t.Fatal("expected error for blocked path")
	}
	data, readErr := os.ReadFile(filepath.Join(memDir, "learnings.jsonl"))
	if readErr != nil {
		t.Fatalf("learnings.jsonl not created: %v", readErr)
	}
	var entry map[string]any
	if jsonErr := json.Unmarshal(data, &entry); jsonErr != nil {
		t.Fatalf("invalid JSON in learnings.jsonl: %v", jsonErr)
	}
	if entry["outcome"] != "rejected" {
		t.Errorf("expected outcome=rejected, got %v", entry["outcome"])
	}
	if !strings.Contains(entry["blocked_path"].(string), "core/") {
		t.Errorf("expected blocked_path to contain core/, got %v", entry["blocked_path"])
	}
}

// TEST-9.3 — Gate: vet/test/build failure causes revert
func TestGate(t *testing.T) {
	t.Parallel()
	// A gate with a command that always succeeds.
	g := evolve.NewGate([]string{"true"}, []string{"true"}, []string{"true"})
	if err := g.Run(t.TempDir()); err != nil {
		t.Fatalf("expected gate to pass: %v", err)
	}
	// A gate where one command fails.
	gFail := evolve.NewGate([]string{"true"}, []string{"false"}, []string{"true"})
	if err := gFail.Run(t.TempDir()); err == nil {
		t.Fatal("expected gate to fail")
	}
}

// TEST-9.4 — Gate: smoke test
func TestSmoke(t *testing.T) {
	t.Parallel()
	// Smoke passes when binary echoes "OK".
	s := evolve.NewSmokeTest("echo", []string{"OK everything"})
	if err := s.Run(); err != nil {
		t.Fatalf("expected smoke to pass: %v", err)
	}
	// Smoke fails when binary echoes something else.
	sFail := evolve.NewSmokeTest("echo", []string{"FAIL"})
	// "FAIL" does not start with OK
	if err := sFail.Run(); err == nil {
		t.Fatal("expected smoke to fail")
	}
}

// TEST-9.5 — Promotion: atomic binary swap
func TestPromote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "current-binary")
	if err := os.WriteFile(src, []byte("newbinary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("oldbinary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := evolve.AtomicSwap(src, dst); err != nil {
		t.Fatalf("AtomicSwap: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "newbinary" {
		t.Errorf("expected newbinary after swap, got %q", string(data))
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src should be gone after rename")
	}
}

// TEST-9.6 — Learnings: append on success and failure
func TestLearnings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ls := evolve.NewLearningsStore(memDir)

	p := evolve.Proposal{Summary: "add feature X", Patches: nil}
	if err := ls.Append(evolve.LearningEntry{
		Timestamp: time.Now(),
		Proposal:  p,
		Outcome:   "applied",
		GitSHA:    "abc123",
	}); err != nil {
		t.Fatalf("Append applied: %v", err)
	}
	if err := ls.Append(evolve.LearningEntry{
		Timestamp:  time.Now(),
		Proposal:   p,
		Outcome:    "reverted",
		GateOutput: "go test failed",
	}); err != nil {
		t.Fatalf("Append reverted: %v", err)
	}

	entries, err := ls.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Outcome != "applied" {
		t.Errorf("expected applied, got %q", entries[0].Outcome)
	}
	if entries[1].Outcome != "reverted" {
		t.Errorf("expected reverted, got %q", entries[1].Outcome)
	}
}

// TEST-9.7 — active_learnings.md synthesis
func TestSynthesis(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ls := evolve.NewLearningsStore(memDir)

	// Recent entry (today).
	recentProposal := evolve.Proposal{Summary: "recent change"}
	if err := ls.Append(evolve.LearningEntry{
		Timestamp: time.Now(),
		Proposal:  recentProposal,
		Outcome:   "applied",
		GitSHA:    "def456",
	}); err != nil {
		t.Fatal(err)
	}
	// Old entry (10 days ago).
	oldProposal := evolve.Proposal{Summary: "old improvement"}
	if err := ls.Append(evolve.LearningEntry{
		Timestamp: time.Now().AddDate(0, 0, -10),
		Proposal:  oldProposal,
		Outcome:   "reverted",
	}); err != nil {
		t.Fatal(err)
	}

	synth := evolve.NewSynthesizer(memDir)
	if err := synth.Synthesize(); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(memDir, "active_learnings.md"))
	if err != nil {
		t.Fatalf("active_learnings.md not created: %v", err)
	}
	content := string(data)
	// Recent entry should appear verbatim.
	if !strings.Contains(content, "recent change") {
		t.Errorf("expected recent entry in active_learnings.md")
	}
	// Old entry should appear as summary (not necessarily verbatim).
	if !strings.Contains(content, "old improvement") && !strings.Contains(content, "reverted") {
		t.Errorf("expected old entry summary in active_learnings.md")
	}
	// Idempotent: run again should not error.
	if err := synth.Synthesize(); err != nil {
		t.Fatalf("second Synthesize: %v", err)
	}
}

// findRepoRoot walks up to find the git repo root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
