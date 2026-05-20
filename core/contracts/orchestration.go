package contracts

import "context"

type PatternRunner interface {
	RunPattern(ctx context.Context, req PatternRequest) (PatternResult, error)
}

type PatternResumer interface {
	ResumePattern(ctx context.Context, checkpointID string, input ResumeInput) (PatternResult, error)
}

type HandoffTarget interface {
	Handoff(ctx context.Context, input HandoffInput) (HandoffResult, error)
}

type PatternRuntime interface {
	PatternRunner
	PatternResumer
	HandoffTarget
}

type PatternRequest struct {
	ID          string
	SessionID   string
	StudentID   string
	Prompt      string
	SkillName   string
	LessonID    string
	PatternHint string
	Context     map[string]any
	Budget      Budget
	Policy      PatternPolicy
	Metadata    map[string]string
}

type PatternPolicy struct {
	AllowSubagents     bool
	AllowParallel      bool
	AllowHandoff       bool
	RequireApproval    bool
	RedactTraces       bool
	MaxHandoffDepth    int
	AllowedPatterns    []string
	AllowedAgents      []string
	AllowedTools       []string
	HumanReviewReasons []string
}

type PatternResult struct {
	Text           string
	ChildText      string
	ParentSummary  string
	Artifacts      []ArtifactRef
	TraceID        string
	CheckpointID   string
	NeedsHuman     bool
	ResumeQuestion string
	Evidence       []EvidenceRecord
	Metadata       map[string]string
}

type ResumeInput struct {
	Text     string
	Approved bool
	Rejected bool
	Data     map[string]any
	Metadata map[string]string
}

type HandoffInput struct {
	ID        string
	SessionID string
	FromAgent string
	ToAgent   string
	Reason    string
	Summary   string
	Prompt    string
	Context   map[string]any
	Budget    Budget
	Policy    PatternPolicy
	Metadata  map[string]string
}

type HandoffResult struct {
	Accepted     bool
	Text         string
	TraceID      string
	CheckpointID string
	Metadata     map[string]string
}

type EvidenceRecord struct {
	Kind      string
	SubjectID string
	Value     map[string]any
	Metadata  map[string]string
}
