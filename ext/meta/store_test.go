package meta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONLStoreAppendsValidRecords(t *testing.T) {
	root := t.TempDir()
	store := NewJSONLStore(root)
	ctx := context.Background()
	ts := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	rec := LineageRecord{ID: "lineage-1", Timestamp: ts, Actor: "test", Event: "created", LessonID: "lesson-1", RunID: "run-1"}
	if err := store.AppendLineage(ctx, rec); err != nil {
		t.Fatalf("append lineage: %v", err)
	}
	if err := store.AppendLineage(ctx, LineageRecord{ID: "lineage-2", Timestamp: ts, Actor: "test", Event: "tested", LessonID: "lesson-1", RunID: "run-1"}); err != nil {
		t.Fatalf("append lineage again: %v", err)
	}
	path := filepath.Join(root, "memory/meta/lineage.jsonl")
	if err := AssertJSONLLines(path); err != nil {
		t.Fatalf("jsonl invalid: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := lineCount(data); got != 2 {
		t.Fatalf("line count = %d, want append-only 2", got)
	}
}

func TestFakeAndJSONLStoresSatisfyEvidenceStore(t *testing.T) {
	var _ EvidenceStore = (*JSONLStore)(nil)
	var _ EvidenceStore = (*FakeStore)(nil)
}

func lineCount(data []byte) int {
	var n int
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}
