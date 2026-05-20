package contracts

import "context"

type ToolCatalog interface {
	ListTools(ctx context.Context) ([]ToolSpec, error)
}

type ToolInvoker interface {
	InvokeTool(ctx context.Context, req ToolInvocation) (ToolResult, error)
}

type ToolRuntime interface {
	ToolCatalog
	ToolInvoker
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema []byte
	Metadata    map[string]string
}

type ToolInvocation struct {
	Name      string
	Arguments map[string]any
	Context   map[string]any
	TimeoutMS int
	Metadata  map[string]string
}

type ToolResult struct {
	Text      string
	Data      map[string]any
	Error     string
	Artifacts []ArtifactRef
	Metadata  map[string]string
}
