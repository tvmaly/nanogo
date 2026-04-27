package slog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/obs"
	slogobs "github.com/tvmaly/nanogo/ext/obs/slog"
)

func TestSlogJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	logger := slogobs.New(slogobs.Config{Format: "json"}, &out)
	if err := logger.Log(context.Background(), obs.Entry{Level: obs.LevelInfo, Message: "hello", Attrs: map[string]any{"kind": "turn.started"}}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
		t.Fatalf("json: %v: %s", err, out.String())
	}
	if rec["msg"] != "hello" || rec["kind"] != "turn.started" {
		t.Fatalf("record = %+v", rec)
	}
}
