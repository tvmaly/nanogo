package evolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Synthesizer regenerates active_learnings.md from learnings.jsonl.
type Synthesizer struct {
	memDir string
}

// NewSynthesizer creates a Synthesizer.
func NewSynthesizer(memDir string) *Synthesizer {
	return &Synthesizer{memDir: memDir}
}

// Synthesize reads learnings.jsonl and rewrites active_learnings.md.
// Entries ≤7 days old appear verbatim; older entries appear as summaries.
func (s *Synthesizer) Synthesize() error {
	ls := NewLearningsStore(s.memDir)
	entries, err := ls.Load()
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -7)
	var recent, old []LearningEntry
	for _, e := range entries {
		if e.Timestamp.After(cutoff) {
			recent = append(recent, e)
		} else {
			old = append(old, e)
		}
	}

	var sb strings.Builder
	sb.WriteString("# Active Learnings\n\n")

	if len(recent) > 0 {
		sb.WriteString("## Recent (last 7 days)\n\n")
		for _, e := range recent {
			fmt.Fprintf(&sb, "- [%s] %s — %s", e.Timestamp.Format("2006-01-02"), e.Proposal.Summary, e.Outcome)
			if e.GitSHA != "" {
				fmt.Fprintf(&sb, " (sha: %s)", e.GitSHA)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(old) > 0 {
		sb.WriteString("## Older summaries\n\n")
		for _, e := range old {
			fmt.Fprintf(&sb, "- [%s] %s — %s\n", e.Timestamp.Format("2006-01-02"), e.Proposal.Summary, e.Outcome)
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(filepath.Join(s.memDir, "active_learnings.md"), []byte(sb.String()), 0o644)
}
