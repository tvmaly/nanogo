package jsonl_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/modules/obs"
	obsjsonl "github.com/tvmaly/nanogo/modules/obs/jsonl"
)

func TestStoreAppendsValidJSONLines(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := obsjsonl.New(obsjsonl.Config{Root: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	rec := validRecord("obs-1")
	if err := store.Append(ctx, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	path := filepath.Join(dir, "observations.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("missing JSONL record")
	}
	var got obs.ObservationRecord
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != "obs-1" || got.Type != "run.start" {
		t.Fatalf("record = %#v", got)
	}
}

func TestStoreRejectsInvalidRecordBeforeAppend(t *testing.T) {
	store, err := obsjsonl.New(obsjsonl.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	if err := store.Append(context.Background(), obs.ObservationRecord{}); !errors.Is(err, obs.ErrInvalidRecord) {
		t.Fatalf("err = %v, want ErrInvalidRecord", err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("data = %q, want empty", data)
	}
}

func TestStorePreservesUnknownAttributes(t *testing.T) {
	rec := validRecord("obs-unknown")
	rec.Unknown = map[string]json.RawMessage{"future_field": json.RawMessage(`{"enabled":true}`)}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded obs.ObservationRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(decoded.Unknown["future_field"]) != `{"enabled":true}` {
		t.Fatalf("unknown = %s", decoded.Unknown["future_field"])
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal decoded: %v", err)
	}
	if !json.Valid(encoded) || !containsJSONField(encoded, "future_field") {
		t.Fatalf("encoded = %s", encoded)
	}
}

func TestStoreClosedAndQueryErrors(t *testing.T) {
	store, err := obsjsonl.New(obsjsonl.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := store.Query(context.Background(), obs.QuerySpec{}); !errors.Is(err, obs.ErrQueryNotImplemented) {
		t.Fatalf("Query err = %v, want ErrQueryNotImplemented", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Append(context.Background(), validRecord("closed")); !errors.Is(err, obs.ErrStoreClosed) {
		t.Fatalf("Append err = %v, want ErrStoreClosed", err)
	}
	if err := store.Flush(); !errors.Is(err, obs.ErrStoreClosed) {
		t.Fatalf("Flush err = %v, want ErrStoreClosed", err)
	}
}

func validRecord(id string) obs.ObservationRecord {
	return obs.ObservationRecord{
		SchemaVersion: obs.SchemaVersion,
		ID:            id,
		Type:          "run.start",
		Time:          time.Unix(10, 0).UTC(),
		Source:        "test",
	}
}

func containsJSONField(data []byte, field string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, ok := raw[field]
	return ok
}
