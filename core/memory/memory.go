// Package memory provides file-backed memory: Store, Consolidator, Dream, Curator.
package memory

import "context"


// Consolidator listens to TurnCompleted events and compresses old messages.
type Consolidator interface {
	// Run starts the consolidator event loop; blocks until ctx is cancelled.
	Run(ctx context.Context) error
}

// Dreamer runs the two-phase dream pass on history.jsonl.
type Dreamer interface {
	// Dream processes unprocessed history entries and updates identity files.
	Dream(ctx context.Context) error
}

// CuratorStore manages the four-tier long-term memory layout.
type CuratorStore interface {
	WriteTopic(slug, content string) error
	ReadTopic(slug string) (string, error)
	EditTopic(slug, old, replacement string) error
	LinkTopics(from, to, relation string) error
	Grep(pattern string) ([]string, error)
	PruneOld(ctx context.Context, pruneDays int, threshold float64) error
	RebuildIndex() error
}
