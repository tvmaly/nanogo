package fake_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/core/tools"
	toolsfake "github.com/tvmaly/nanogo/core/tools/fake"
)

func TestToolAndSource(t *testing.T) {
	tool := toolsfake.New("fake", "ok")
	if tool.Name() != "fake" || !json.Valid(tool.Schema()) {
		t.Fatalf("bad tool")
	}
	got, err := tool.Call(context.Background(), json.RawMessage(`{"x":1}`))
	if err != nil || got != "ok" || len(tool.Calls) != 1 {
		t.Fatalf("call = %q %v calls=%d", got, err, len(tool.Calls))
	}
	ts, err := toolsfake.NewSource(tool).Tools(context.Background(), tools.TurnInfo{})
	if err != nil || len(ts) != 1 {
		t.Fatalf("source = %v %v", ts, err)
	}
}
