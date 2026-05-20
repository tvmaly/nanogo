package agent_test

import (
	"testing"

	"github.com/tvmaly/nanogo/core/agent"
	"github.com/tvmaly/nanogo/core/contracts"
)

func TestSubagentRunnerImplementsSubagentSpawner(t *testing.T) {
	var _ contracts.SubagentSpawner = (*agent.SubagentRunner)(nil)
}
