package evolve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var blockedPrefixes = []string{"core/", "ext/evolve/"}

// IsBlocked returns true if the given path is in a protected directory.
func IsBlocked(path string) bool {
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// ValidatePaths checks proposal patches for blocked paths. On rejection it
// appends an entry to <memDir>/learnings.jsonl and returns an error.
func ValidatePaths(p Proposal, memDir string) error {
	for _, patch := range p.Patches {
		if IsBlocked(patch.Path) {
			entry := map[string]any{
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"outcome":      "rejected",
				"blocked_path": patch.Path,
				"proposal":     p.Summary,
			}
			_ = appendLearning(memDir, entry)
			return fmt.Errorf("blocked path: %s", patch.Path)
		}
	}
	return nil
}

func appendLearning(memDir string, entry map[string]any) error {
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(memDir, "learnings.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}
