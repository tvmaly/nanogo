package evolve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Sandbox wraps a git worktree used for evolve proposals.
type Sandbox struct {
	dir     string
	repoDir string
}

// NewSandbox creates a git worktree at <repoDir>/evolve-work at the current HEAD.
func NewSandbox(repoDir string) (*Sandbox, error) {
	dir := filepath.Join(repoDir, "evolve-work")
	// Remove stale worktree if it exists.
	_ = os.RemoveAll(dir)
	cmd := exec.Command("git", "-C", repoDir, "worktree", "add", "--detach", dir, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return &Sandbox{dir: dir, repoDir: repoDir}, nil
}

// Dir returns the path to the worktree directory.
func (s *Sandbox) Dir() string { return s.dir }

// Close removes the worktree and prunes git's internal reference.
func (s *Sandbox) Close() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	// Prune the worktree reference; ignore errors (may already be gone).
	_ = exec.Command("git", "-C", s.repoDir, "worktree", "prune").Run()
	return nil
}
