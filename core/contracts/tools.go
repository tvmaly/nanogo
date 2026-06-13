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
	Name, Description string
	InputSchema       []byte
	Metadata          map[string]string
}

type ToolInvocation struct {
	Name               string
	Arguments, Context map[string]any
	TimeoutMS          int
	Metadata           map[string]string
}

type ToolResult struct {
	Text, Error string
	Data        map[string]any
	Artifacts   []ArtifactRef
	Metadata    map[string]string
}
