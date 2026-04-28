package evolve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LearningsStore appends and reads learnings.jsonl.
type LearningsStore struct {
	memDir string
}

// NewLearningsStore creates a LearningsStore targeting <memDir>/learnings.jsonl.
func NewLearningsStore(memDir string) *LearningsStore {
	return &LearningsStore{memDir: memDir}
}

type learningRecord struct {
	Timestamp  string   `json:"timestamp"`
	Proposal   Proposal `json:"proposal"`
	Outcome    string   `json:"outcome"`
	GateOutput string   `json:"gate_output,omitempty"`
	GitSHA     string   `json:"git_sha,omitempty"`
}

// Append writes one entry to learnings.jsonl.
func (ls *LearningsStore) Append(e LearningEntry) error {
	if err := os.MkdirAll(ls.memDir, 0o755); err != nil {
		return err
	}
	rec := learningRecord{
		Timestamp:  e.Timestamp.UTC().Format(time.RFC3339),
		Proposal:   e.Proposal,
		Outcome:    e.Outcome,
		GateOutput: e.GateOutput,
		GitSHA:     e.GitSHA,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(ls.memDir, "learnings.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}

// Load reads all entries from learnings.jsonl.
func (ls *LearningsStore) Load() ([]LearningEntry, error) {
	f, err := os.Open(filepath.Join(ls.memDir, "learnings.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var entries []LearningEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec learningRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
		entries = append(entries, LearningEntry{
			Timestamp:  ts,
			Proposal:   rec.Proposal,
			Outcome:    rec.Outcome,
			GateOutput: rec.GateOutput,
			GitSHA:     rec.GitSHA,
		})
	}
	return entries, sc.Err()
}
