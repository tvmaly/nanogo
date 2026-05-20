package graph_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/ext/agentpatterns/graph"
)

func TestCheckpointStoreWritesAndLoadsJSONL(t *testing.T) {
	store := graph.NewCheckpointStore(filepath.Join(t.TempDir(), "checkpoints.jsonl"))
	rec := graph.Checkpoint{RunID: "run-1", SessionID: "s1", Pattern: "sequential", Node: "step-2", Step: 2, ResumeKind: "human_input"}
	id, err := store.Save(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Version != 1 || got.Pattern != "sequential" || got.Node != "step-2" {
		t.Fatalf("checkpoint = %#v", got)
	}
}
