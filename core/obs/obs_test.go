package obs_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/obs"
)

func TestNoopDefaults(t *testing.T) {
	obs.Reset()
	if err := obs.Log(context.Background(), obs.Entry{Level: obs.LevelInfo, Message: "ignored"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	span := obs.Trace.Start(context.Background(), "noop")
	span.End(nil)
}

func TestMultiAdapter(t *testing.T) {
	obs.Reset()
	var a, b bytes.Buffer
	obs.SetLoggers(obs.NewTextLogger(&a), obs.NewTextLogger(&b))
	if err := obs.Log(context.Background(), obs.Entry{Level: obs.LevelInfo, Message: "hello"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if !bytes.Contains(a.Bytes(), []byte("hello")) || !bytes.Contains(b.Bytes(), []byte("hello")) {
		t.Fatalf("fanout failed: %q %q", a.String(), b.String())
	}
}
