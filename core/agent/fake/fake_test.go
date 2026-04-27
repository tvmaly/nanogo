package fake_test

import (
	"context"
	"testing"

	agentfake "github.com/tvmaly/nanogo/core/agent/fake"
	"github.com/tvmaly/nanogo/core/tools"
)

func TestRunner(t *testing.T) {
	r := agentfake.New("done")
	got, err := r.RunSubagent(context.Background(), tools.SubagentOpts{Goal: "g"})
	if err != nil || got != "done" || r.LastOpts.Goal != "g" {
		t.Fatalf("got %q %v opts=%+v", got, err, r.LastOpts)
	}
}
