package evolve

import "time"

// Proposal represents a proposed code change from the evolve agent.
type Proposal struct {
	Summary string
	Patches []Patch
}

// Patch is a single file change.
type Patch struct {
	Path    string
	Content string
}

// LearningEntry is one record appended to learnings.jsonl.
type LearningEntry struct {
	Timestamp  time.Time
	Proposal   Proposal
	Outcome    string // "applied" | "reverted" | "rejected"
	GateOutput string
	GitSHA     string
}
