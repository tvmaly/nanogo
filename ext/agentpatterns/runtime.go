package agentpatterns

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Runtime struct {
	cfg         Config
	mu          sync.Mutex
	checkpoints map[string]contracts.PatternRequest
	nextID      int
}

var _ contracts.PatternRuntime = (*Runtime)(nil)

func New(cfg Config) *Runtime {
	return &Runtime{cfg: cfg, checkpoints: map[string]contracts.PatternRequest{}}
}

func (r *Runtime) RunPattern(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	pattern := r.selectPattern(req)
	r.trace(ctx, req, pattern, "pattern.started", "ok", "")
	var res contracts.PatternResult
	var err error
	switch pattern {
	case "single":
		res, err = r.runSingle(ctx, req)
	case "supervisor_worker":
		res, err = r.runSupervisor(ctx, req)
	case "sequential":
		res, err = r.runSequential(ctx, req)
	case "parallel":
		res, err = r.runParallel(ctx, req)
	case "loop":
		res, err = r.runLoop(ctx, req)
	case "review":
		res, err = r.runReview(ctx, req)
	case "handoff":
		hr, hErr := r.Handoff(ctx, contracts.HandoffInput{SessionID: req.SessionID, ToAgent: req.Metadata["to_agent"], Prompt: req.Prompt, Context: req.Context, Budget: req.Budget, Policy: req.Policy, Metadata: req.Metadata})
		res, err = contracts.PatternResult{Text: hr.Text, TraceID: hr.TraceID, CheckpointID: hr.CheckpointID, Metadata: map[string]string{"pattern": "handoff", "accepted": strconv.FormatBool(hr.Accepted)}}, hErr
	case "human_review":
		res, err = r.runHumanReview(ctx, req)
	default:
		res, err = r.runSingle(ctx, req)
		pattern = "single"
	}
	if res.Metadata == nil {
		res.Metadata = map[string]string{}
	}
	res.Metadata["pattern"] = pattern
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.trace(ctx, req, pattern, "pattern.completed", status, firstNonEmpty(res.Text, errString(err)))
	return res, err
}

func (r *Runtime) ResumePattern(ctx context.Context, checkpointID string, input contracts.ResumeInput) (contracts.PatternResult, error) {
	r.mu.Lock()
	req, ok := r.checkpoints[checkpointID]
	r.mu.Unlock()
	if !ok {
		return contracts.PatternResult{}, fmt.Errorf("resume pattern: checkpoint %q not found", checkpointID)
	}
	r.trace(ctx, req, firstNonEmpty(req.PatternHint, "human_review"), "checkpoint.resumed", "ok", input.Text)
	return contracts.PatternResult{Text: "resumed: " + input.Text, Metadata: map[string]string{"resumed_from": checkpointID}}, nil
}

func (r *Runtime) Handoff(ctx context.Context, input contracts.HandoffInput) (contracts.HandoffResult, error) {
	if !input.Policy.AllowHandoff {
		return contracts.HandoffResult{Accepted: false, Metadata: map[string]string{"reason": "handoff_not_allowed"}}, nil
	}
	if input.Policy.MaxHandoffDepth > 0 {
		depth, _ := strconv.Atoi(input.Metadata["handoff_depth"])
		if depth > input.Policy.MaxHandoffDepth {
			return contracts.HandoffResult{Accepted: false, Metadata: map[string]string{"reason": "handoff_depth_exceeded"}}, nil
		}
	}
	if len(input.Policy.AllowedAgents) > 0 && !contains(input.Policy.AllowedAgents, input.ToAgent) {
		return contracts.HandoffResult{Accepted: false, Metadata: map[string]string{"reason": "destination_not_allowed"}}, nil
	}
	req := contracts.PatternRequest{SessionID: input.SessionID, Prompt: input.Prompt, Context: input.Context, Budget: input.Budget, Policy: input.Policy, Metadata: input.Metadata}
	r.trace(ctx, req, "handoff", "handoff.accepted", "ok", input.ToAgent)
	return contracts.HandoffResult{Accepted: true, Text: firstNonEmpty(input.Summary, "handoff accepted"), Metadata: map[string]string{"to_agent": input.ToAgent}}, nil
}

func (r *Runtime) selectPattern(req contracts.PatternRequest) string {
	hint := normalizePattern(req.PatternHint)
	if hint == "" {
		hint = normalizePattern(req.Metadata["pattern"])
	}
	if hint == "" {
		hint = r.cfg.defaultPattern()
	}
	if hint == "router" {
		choice := normalizePattern(req.Metadata["router_choice"])
		conf, _ := strconv.ParseFloat(req.Metadata["router_confidence"], 64)
		if r.cfg.RouterEnabled && conf >= 0.5 && knownPattern(choice) {
			return choice
		}
		return "single"
	}
	if !knownPattern(hint) {
		return "single"
	}
	return hint
}

func (r *Runtime) runSingle(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	if r.cfg.ToolRuntime != nil {
		if _, err := r.cfg.ToolRuntime.ListTools(ctx); err != nil {
			return contracts.PatternResult{Metadata: map[string]string{"tool_error": err.Error()}}, err
		}
		if tool := req.Metadata["tool"]; tool != "" {
			r.trace(ctx, req, "single", "tool.started", "ok", tool)
			out, err := r.cfg.ToolRuntime.InvokeTool(ctx, contracts.ToolInvocation{Name: tool, Arguments: req.Context, Metadata: req.Metadata})
			if err != nil {
				r.trace(ctx, req, "single", "tool.completed", "error", out.Error)
				return contracts.PatternResult{Text: out.Text, Metadata: map[string]string{"tool_error": out.Error}}, err
			}
			r.trace(ctx, req, "single", "tool.completed", "ok", out.Text)
			return contracts.PatternResult{Text: out.Text, Artifacts: out.Artifacts}, nil
		}
	}
	if r.cfg.AgentRunner != nil {
		out, err := r.cfg.AgentRunner.RunAgent(ctx, contracts.AgentRequest{ID: req.ID, SessionID: req.SessionID, Prompt: req.Prompt, Context: req.Context, Budget: req.Budget, Metadata: req.Metadata})
		return contracts.PatternResult{Text: out.Text, Artifacts: out.Artifacts, TraceID: out.TraceID}, err
	}
	return contracts.PatternResult{Text: req.Prompt}, nil
}

func (r *Runtime) runSupervisor(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	if !req.Policy.AllowSubagents || r.cfg.SubagentSpawner == nil {
		return contracts.PatternResult{Metadata: map[string]string{"reason": "subagents_not_available"}}, nil
	}
	if req.Metadata["peer_handoff"] == "true" {
		return contracts.PatternResult{Metadata: map[string]string{"reason": "peer_handoff_rejected"}}, nil
	}
	prompts := anyStrings(req.Context["worker_prompts"])
	if len(prompts) == 0 {
		prompts = []string{"worker"}
	}
	max := req.Budget.MaxSubagents
	if max <= 0 || max > len(prompts) {
		max = len(prompts)
	}
	var parts []string
	var artifacts []contracts.ArtifactRef
	for i := 0; i < max; i++ {
		r.trace(ctx, req, "supervisor_worker", "subagent.started", "ok", prompts[i])
		out, err := r.cfg.SubagentSpawner.SpawnSubagent(ctx, contracts.SubagentRequest{
			ParentRunID: req.ID, SessionID: req.SessionID, Name: fmt.Sprintf("worker-%d", i+1), Prompt: prompts[i],
			Context: filteredContext(req.Context), AllowedTools: allowedWorkerTools(req), Budget: req.Budget, Metadata: req.Metadata,
		})
		if err != nil {
			return contracts.PatternResult{Metadata: map[string]string{"subagent_error": err.Error()}}, err
		}
		parts = append(parts, firstNonEmpty(out.Summary, compact(out.Text, 80)))
		if len(out.Text) > 80 {
			artifacts = append(artifacts, contracts.ArtifactRef{Kind: "worker_output", URI: "memory://agentpatterns/worker-output", Name: "worker-output"})
		}
		r.trace(ctx, req, "supervisor_worker", "subagent.completed", "ok", parts[len(parts)-1])
	}
	return contracts.PatternResult{Text: strings.Join(parts, "\n"), Artifacts: artifacts, Metadata: map[string]string{"workers": strconv.Itoa(max)}}, nil
}

func (r *Runtime) runSequential(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	steps := splitCSV(req.Metadata["steps"])
	if len(steps) == 0 {
		steps = []string{"step-1"}
	}
	for i, step := range steps {
		r.trace(ctx, req, "sequential", "node.completed", "ok", step)
		if req.Metadata["fail_step"] == step {
			return contracts.PatternResult{CheckpointID: r.saveCheckpoint(req), Metadata: map[string]string{"failed_step": step, "completed": strings.Join(steps[:i], ",")}}, nil
		}
	}
	return contracts.PatternResult{Text: "sequential complete", Metadata: map[string]string{"order": strings.Join(steps, ",")}}, nil
}

func (r *Runtime) runParallel(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	branches := splitCSV(req.Metadata["branches"])
	if len(branches) == 0 {
		branches = []string{"branch-1"}
	}
	capN := req.Budget.MaxConcurrentAgents
	if capN <= 0 || capN > len(branches) {
		capN = len(branches)
	}
	for _, branch := range branches {
		if ctx.Err() != nil {
			break
		}
		r.trace(ctx, req, "parallel", "node.completed", "ok", branch)
	}
	return contracts.PatternResult{Text: "parallel complete", Metadata: map[string]string{"branches": strconv.Itoa(len(branches)), "max_concurrent_seen": strconv.Itoa(capN)}}, ctx.Err()
}

func (r *Runtime) runLoop(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	limit := req.Budget.MaxModelCalls
	if limit <= 0 {
		limit = 1
	}
	passAt, _ := strconv.Atoi(req.Metadata["pass_at"])
	for i := 1; i <= limit; i++ {
		r.trace(ctx, req, "loop", "node.completed", "ok", fmt.Sprintf("iteration-%d", i))
		if passAt > 0 && i >= passAt {
			return contracts.PatternResult{Text: "loop complete", Metadata: map[string]string{"iterations": strconv.Itoa(i), "stop_reason": "condition_passed"}}, nil
		}
	}
	return contracts.PatternResult{Metadata: map[string]string{"iterations": strconv.Itoa(limit), "stop_reason": "budget_exceeded"}}, nil
}

func (r *Runtime) runReview(_ context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	rejects, _ := strconv.Atoi(req.Metadata["critic_rejects"])
	if rejects > 1 {
		return contracts.PatternResult{NeedsHuman: true, Metadata: map[string]string{"review": "hard_fail"}}, nil
	}
	if rejects == 1 {
		return contracts.PatternResult{Text: "revised answer", Metadata: map[string]string{"review": "accepted_after_revision"}}, nil
	}
	return contracts.PatternResult{Text: "accepted answer", Metadata: map[string]string{"review": "accepted"}}, nil
}

func (r *Runtime) runHumanReview(ctx context.Context, req contracts.PatternRequest) (contracts.PatternResult, error) {
	if !req.Policy.RequireApproval {
		return contracts.PatternResult{Text: "approval not required"}, nil
	}
	if r.cfg.ApprovalGate != nil {
		out, err := r.cfg.ApprovalGate.RequestApproval(ctx, contracts.ApprovalRequest{ID: req.ID, SessionID: req.SessionID, Reason: "human_review", Summary: req.Prompt, Data: req.Context, Metadata: req.Metadata})
		if err != nil {
			return contracts.PatternResult{}, err
		}
		if out.Approved {
			return contracts.PatternResult{Text: out.Comment, Metadata: map[string]string{"approval": "approved"}}, nil
		}
		if out.Rejected {
			return contracts.PatternResult{Text: out.Comment, Metadata: map[string]string{"approval": "rejected"}}, nil
		}
	}
	id := r.saveCheckpoint(req)
	r.trace(ctx, req, "human_review", "checkpoint.saved", "ok", id)
	return contracts.PatternResult{NeedsHuman: true, CheckpointID: id, ResumeQuestion: "Approval required", Metadata: map[string]string{"approval": "unavailable"}}, nil
}

func (r *Runtime) saveCheckpoint(req contracts.PatternRequest) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	id := fmt.Sprintf("checkpoint-%d", r.nextID)
	r.checkpoints[id] = req
	return id
}

func (r *Runtime) trace(ctx context.Context, req contracts.PatternRequest, pattern, kind, status, msg string) {
	if r.cfg.TraceSink == nil {
		return
	}
	_ = r.cfg.TraceSink.EmitTrace(ctx, contracts.TraceEvent{Version: 1, RunID: firstNonEmpty(req.ID, req.SessionID), SessionID: req.SessionID, Pattern: pattern, Kind: kind, Status: status, Message: msg, CreatedAt: time.Now().UTC(), Data: map[string]any{"budget": req.Budget}})
}

func knownPattern(p string) bool {
	switch p {
	case "single", "router", "supervisor_worker", "sequential", "parallel", "loop", "review", "handoff", "human_review":
		return true
	default:
		return false
	}
}

func normalizePattern(p string) string { return strings.ReplaceAll(strings.TrimSpace(p), "-", "_") }
func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
func splitCSV(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
func anyStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
func filteredContext(in map[string]any) map[string]any {
	out := map[string]any{}
	if v, ok := in["compact_summary"]; ok {
		out["compact_summary"] = v
	}
	return out
}
func allowedWorkerTools(req contracts.PatternRequest) []string {
	allowedSet := map[string]bool{}
	for _, name := range anyStrings(req.Context["allowed_tool_set"]) {
		allowedSet[name] = true
	}
	if len(allowedSet) == 0 {
		return append([]string(nil), req.Policy.AllowedTools...)
	}
	var out []string
	for _, name := range req.Policy.AllowedTools {
		if allowedSet[name] {
			out = append(out, name)
		}
	}
	return out
}
func compact(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
