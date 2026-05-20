package fake

import (
	"context"

	"github.com/tvmaly/nanogo/core/contracts"
)

var _ contracts.AgentRunner = (*AgentRunner)(nil)
var _ contracts.SubagentSpawner = (*SubagentSpawner)(nil)
var _ contracts.ToolCatalog = (*ToolRuntime)(nil)
var _ contracts.ToolInvoker = (*ToolRuntime)(nil)
var _ contracts.ToolRuntime = (*ToolRuntime)(nil)
var _ contracts.PatternRunner = (*PatternRuntime)(nil)
var _ contracts.PatternResumer = (*PatternRuntime)(nil)
var _ contracts.HandoffTarget = (*PatternRuntime)(nil)
var _ contracts.PatternRuntime = (*PatternRuntime)(nil)
var _ contracts.TraceSink = (*TraceSink)(nil)
var _ contracts.ApprovalGate = (*ApprovalGate)(nil)

type AgentRunner struct {
	Requests []contracts.AgentRequest
	Result   contracts.AgentResult
	Err      error
}

func (r *AgentRunner) RunAgent(_ context.Context, req contracts.AgentRequest) (contracts.AgentResult, error) {
	r.Requests = append(r.Requests, req)
	return r.Result, r.Err
}

type SubagentSpawner struct {
	Requests []contracts.SubagentRequest
	Result   contracts.SubagentResult
	Err      error
}

func (s *SubagentSpawner) SpawnSubagent(_ context.Context, req contracts.SubagentRequest) (contracts.SubagentResult, error) {
	s.Requests = append(s.Requests, req)
	return s.Result, s.Err
}

type ToolRuntime struct {
	Specs       []contracts.ToolSpec
	Invocations []contracts.ToolInvocation
	Result      contracts.ToolResult
	ListErr     error
	InvokeErr   error
}

func (r *ToolRuntime) ListTools(context.Context) ([]contracts.ToolSpec, error) {
	return append([]contracts.ToolSpec(nil), r.Specs...), r.ListErr
}

func (r *ToolRuntime) InvokeTool(_ context.Context, req contracts.ToolInvocation) (contracts.ToolResult, error) {
	r.Invocations = append(r.Invocations, req)
	return r.Result, r.InvokeErr
}

type PatternRuntime struct {
	RunRequests    []contracts.PatternRequest
	ResumeRequests []ResumeRequest
	HandoffInputs  []contracts.HandoffInput
	RunResult      contracts.PatternResult
	ResumeResult   contracts.PatternResult
	HandoffResult  contracts.HandoffResult
	RunErr         error
	ResumeErr      error
	HandoffErr     error
}

type ResumeRequest struct {
	CheckpointID string
	Input        contracts.ResumeInput
}

func (r *PatternRuntime) RunPattern(_ context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	r.RunRequests = append(r.RunRequests, req)
	return r.RunResult, r.RunErr
}

func (r *PatternRuntime) ResumePattern(_ context.Context, checkpointID string, input contracts.ResumeInput) (contracts.PatternResult, error) {
	r.ResumeRequests = append(r.ResumeRequests, ResumeRequest{CheckpointID: checkpointID, Input: input})
	return r.ResumeResult, r.ResumeErr
}

func (r *PatternRuntime) Handoff(_ context.Context, input contracts.HandoffInput) (contracts.HandoffResult, error) {
	r.HandoffInputs = append(r.HandoffInputs, input)
	return r.HandoffResult, r.HandoffErr
}

type TraceSink struct {
	Events []contracts.TraceEvent
	Err    error
}

func (s *TraceSink) EmitTrace(_ context.Context, event contracts.TraceEvent) error {
	s.Events = append(s.Events, event)
	return s.Err
}

type ApprovalGate struct {
	Requests []contracts.ApprovalRequest
	Result   contracts.ApprovalResult
	Err      error
}

func (g *ApprovalGate) RequestApproval(_ context.Context, req contracts.ApprovalRequest) (contracts.ApprovalResult, error) {
	g.Requests = append(g.Requests, req)
	return g.Result, g.Err
}
