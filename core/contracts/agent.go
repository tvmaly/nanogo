package contracts

import (
	"context"
	"time"
)

type AgentRunner interface {
	RunAgent(ctx context.Context, req AgentRequest) (AgentResult, error)
}

type SubagentSpawner interface {
	SpawnSubagent(ctx context.Context, req SubagentRequest) (SubagentResult, error)
}

type AgentRequest struct {
	ID        string
	SessionID string
	Prompt    string
	System    string
	Context   map[string]any
	Tools     []ToolSpec
	Budget    Budget
	Metadata  map[string]string
}

type AgentResult struct {
	Text      string
	Artifacts []ArtifactRef
	TraceID   string
	Metadata  map[string]string
}

type SubagentRequest struct {
	ID           string
	ParentRunID  string
	SessionID    string
	Name         string
	Role         string
	Prompt       string
	Context      map[string]any
	AllowedTools []string
	Budget       Budget
	Metadata     map[string]string
}

type SubagentResult struct {
	Text      string
	Summary   string
	Artifacts []ArtifactRef
	TraceID   string
	Metadata  map[string]string
}

type Budget struct {
	MaxModelCalls       int
	MaxToolCalls        int
	MaxSubagents        int
	MaxConcurrentAgents int
	MaxTokensHint       int
	MaxDuration         time.Duration
}

type ArtifactRef struct {
	Kind string
	URI  string
	Name string
	Meta map[string]string
}
