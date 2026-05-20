package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tvmaly/nanogo/core/contracts"
)

type ContractRuntime struct {
	Source Source
}

var _ contracts.ToolCatalog = (*ContractRuntime)(nil)
var _ contracts.ToolInvoker = (*ContractRuntime)(nil)
var _ contracts.ToolRuntime = (*ContractRuntime)(nil)

func NewContractRuntime(src Source) *ContractRuntime {
	return &ContractRuntime{Source: src}
}

func (r *ContractRuntime) ListTools(ctx context.Context) ([]contracts.ToolSpec, error) {
	if r == nil || r.Source == nil {
		return nil, nil
	}
	list, err := r.Source.Tools(ctx, TurnInfo{})
	if err != nil {
		return nil, err
	}
	specs := make([]contracts.ToolSpec, 0, len(list))
	for _, tool := range list {
		specs = append(specs, contracts.ToolSpec{
			Name:        tool.Name(),
			Description: toolDescription(tool.Schema()),
			InputSchema: append([]byte(nil), tool.Schema()...),
			Metadata:    map[string]string{"schema_format": "openai_function"},
		})
	}
	return specs, nil
}

func (r *ContractRuntime) InvokeTool(ctx context.Context, req contracts.ToolInvocation) (contracts.ToolResult, error) {
	if r == nil || r.Source == nil {
		return contracts.ToolResult{}, fmt.Errorf("invoke tool %q: no source configured", req.Name)
	}
	list, err := r.Source.Tools(ctx, TurnInfo{})
	if err != nil {
		return contracts.ToolResult{}, err
	}
	for _, tool := range list {
		if tool.Name() != req.Name {
			continue
		}
		args, err := json.Marshal(req.Arguments)
		if err != nil {
			return contracts.ToolResult{}, fmt.Errorf("invoke tool %q: marshal arguments: %w", req.Name, err)
		}
		text, err := tool.Call(ctx, args)
		if err != nil {
			return contracts.ToolResult{Error: err.Error()}, fmt.Errorf("invoke tool %q: %w", req.Name, err)
		}
		return contracts.ToolResult{Text: text}, nil
	}
	return contracts.ToolResult{}, fmt.Errorf("invoke tool %q: tool not found", req.Name)
}

type SubagentRunnerAdapter struct {
	Spawner contracts.SubagentSpawner
}

var _ Runner = (*SubagentRunnerAdapter)(nil)

func NewSubagentRunnerAdapter(spawner contracts.SubagentSpawner) *SubagentRunnerAdapter {
	return &SubagentRunnerAdapter{Spawner: spawner}
}

func (a *SubagentRunnerAdapter) RunSubagent(ctx context.Context, opts SubagentOpts) (string, error) {
	if a == nil || a.Spawner == nil {
		return "", fmt.Errorf("subagent runner: no spawner configured")
	}
	result, err := a.Spawner.SpawnSubagent(ctx, contracts.SubagentRequest{
		ParentRunID:  opts.ParentSession,
		SessionID:    opts.ParentSession,
		Role:         opts.Role,
		Prompt:       opts.Goal,
		AllowedTools: append([]string(nil), opts.Tools...),
		Metadata:     subagentMetadata(opts),
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func toolDescription(schema json.RawMessage) string {
	var openAI struct {
		Function struct {
			Description string `json:"description"`
		} `json:"function"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(schema, &openAI); err != nil {
		return ""
	}
	if openAI.Function.Description != "" {
		return openAI.Function.Description
	}
	return openAI.Description
}

func subagentMetadata(opts SubagentOpts) map[string]string {
	metadata := map[string]string{}
	if opts.Model != "" {
		metadata["model"] = opts.Model
	}
	return metadata
}
