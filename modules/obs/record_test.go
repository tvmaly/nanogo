package obs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/modules/obs"
)

func TestObservationRecordValidation(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	rec := obs.ObservationRecord{
		SchemaVersion: obs.SchemaVersion,
		ID:            "obs-1",
		Type:          "run.start",
		Time:          now,
		Source:        "test",
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	rec.ID = ""
	err := rec.Validate()
	if !errors.Is(err, obs.ErrInvalidRecord) {
		t.Fatalf("err = %v, want ErrInvalidRecord", err)
	}
}

func TestFakeStoreValidatesAndQueriesRecords(t *testing.T) {
	ctx := context.Background()
	store := obs.NewFakeStore()
	now := time.Unix(10, 0).UTC()
	rec := obs.ObservationRecord{
		SchemaVersion: obs.SchemaVersion,
		ID:            "obs-1",
		Type:          "artifact.created",
		Time:          now,
		Source:        "test",
		Artifacts:     []obs.ArtifactRef{{Kind: "file", URI: "workspace/lessons/a.md"}},
	}
	if err := store.Append(ctx, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := store.Query(ctx, obs.QuerySpec{Types: []string{"artifact.created"}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got.Records) != 1 || got.Records[0].ID != "obs-1" {
		t.Fatalf("records = %#v", got.Records)
	}

	if err := store.Append(ctx, obs.ObservationRecord{}); !errors.Is(err, obs.ErrInvalidRecord) {
		t.Fatalf("invalid append err = %v", err)
	}
}
