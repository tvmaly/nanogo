package agentpatterns

import "github.com/tvmaly/nanogo/core/contracts"

type Config struct {
	Enabled         bool
	DefaultPattern  string
	RouterEnabled   bool
	AgentRunner     contracts.AgentRunner
	ToolRuntime     contracts.ToolRuntime
	SubagentSpawner contracts.SubagentSpawner
	TraceSink       contracts.TraceSink
	ApprovalGate    contracts.ApprovalGate
}

func (c Config) defaultPattern() string {
	if c.DefaultPattern != "" {
		return c.DefaultPattern
	}
	return "single"
}
