package browser

import (
	"context"
	"time"
)

type SessionID string
type TabID string
type FrameID string
type Ref string

type Controller interface {
	Health(context.Context) (Health, error)
	Start(context.Context, StartRequest) (Session, error)
	Connect(context.Context, ConnectRequest) (Session, error)
	Close(context.Context, CloseRequest) error
	Navigate(context.Context, NavigateRequest) (PageState, error)
	Snapshot(context.Context, SnapshotRequest) (Snapshot, error)
	Text(context.Context, TextRequest) (TextResult, error)
	Screenshot(context.Context, ScreenshotRequest) (Artifact, error)
	PDF(context.Context, PDFRequest) (Artifact, error)
	Act(context.Context, ActionRequest) (ActionResult, error)
	Eval(context.Context, EvalRequest) (EvalResult, error)
	Wait(context.Context, WaitRequest) (WaitResult, error)
	Tabs(context.Context, TabsRequest) (TabsResult, error)
	MediaSeek(context.Context, MediaSeekRequest) (MediaSeekResult, error)
}

type Health struct {
	OK           bool     `json:"ok"`
	Driver       string   `json:"driver,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type StartRequest struct {
	SessionName    string   `json:"session_name,omitempty"`
	Headed         bool     `json:"headed"`
	Profile        string   `json:"profile,omitempty"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	FileRoots      []string `json:"allow_file_roots,omitempty"`
}

type ConnectRequest struct {
	Endpoint string `json:"endpoint"`
}

type Session struct {
	ID               SessionID         `json:"session_id"`
	Name             string            `json:"name,omitempty"`
	ActiveTabID      TabID             `json:"tab_id,omitempty"`
	Headed           bool              `json:"headed"`
	Capabilities     []string          `json:"capabilities,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	LessonEventNonce string            `json:"lesson_event_nonce,omitempty"`
	CreatedAt        time.Time         `json:"created_at,omitempty"`
	LastUsedAt       time.Time         `json:"last_used_at,omitempty"`
}

type CloseRequest struct {
	SessionID    SessionID `json:"session_id"`
	TabID        TabID     `json:"tab_id,omitempty"`
	CloseSession bool      `json:"close_session"`
	Reason       string    `json:"reason,omitempty"`
}

type NavigateRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	URL       string    `json:"url"`
	WaitUntil string    `json:"wait_until,omitempty"`
	TimeoutMS int       `json:"timeout_ms,omitempty"`
}

type PageState struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id"`
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	Status    string    `json:"status"`
	Version   int64     `json:"version,omitempty"`
}

type SnapshotRequest struct {
	SessionID       SessionID `json:"session_id"`
	TabID           TabID     `json:"tab_id,omitempty"`
	InteractiveOnly bool      `json:"interactive_only"`
	IncludeIFrames  bool      `json:"include_iframes"`
	MaxDepth        int       `json:"max_depth,omitempty"`
	MaxOutputBytes  int       `json:"max_output_bytes,omitempty"`
}

type Snapshot struct {
	SessionID         SessionID      `json:"session_id"`
	TabID             TabID          `json:"tab_id"`
	SnapshotID        string         `json:"snapshot_id"`
	Version           int64          `json:"version"`
	Text              string         `json:"text,omitempty"`
	Nodes             []SnapshotNode `json:"nodes,omitempty"`
	Frames            []FrameInfo    `json:"frames,omitempty"`
	SnapshotTruncated bool           `json:"snapshot_truncated,omitempty"`
	TruncationReason  string         `json:"truncation_reason,omitempty"`
}

type SnapshotNode struct {
	Ref        Ref               `json:"ref"`
	AdapterRef string            `json:"adapter_ref,omitempty"`
	Role       string            `json:"role,omitempty"`
	Label      string            `json:"label,omitempty"`
	Text       string            `json:"text,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type FrameInfo struct {
	ID          FrameID `json:"frame_id,omitempty"`
	URL         string  `json:"url,omitempty"`
	CrossOrigin bool    `json:"cross_origin,omitempty"`
}

type Target struct {
	Ref      Ref     `json:"ref,omitempty"`
	Selector string  `json:"selector,omitempty"`
	Text     string  `json:"text,omitempty"`
	FrameID  FrameID `json:"frame_id,omitempty"`
}

type ActionKind string

const (
	ActionClick  ActionKind = "click"
	ActionFill   ActionKind = "fill"
	ActionType   ActionKind = "type"
	ActionPress  ActionKind = "press"
	ActionHover  ActionKind = "hover"
	ActionSelect ActionKind = "select"
	ActionScroll ActionKind = "scroll"
	ActionUpload ActionKind = "upload"
	ActionDrag   ActionKind = "drag"
)

type ActionRequest struct {
	SessionID SessionID  `json:"session_id"`
	TabID     TabID      `json:"tab_id,omitempty"`
	Target    Target     `json:"target"`
	Kind      ActionKind `json:"kind"`
	Value     string     `json:"value,omitempty"`
	TimeoutMS int        `json:"timeout_ms,omitempty"`
}

type ActionResult struct {
	Success     bool `json:"success"`
	PageChanged bool `json:"page_changed,omitempty"`
	StaleRefs   bool `json:"stale_refs,omitempty"`
}

type TextRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	Target    Target    `json:"target,omitempty"`
}

type TextResult struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

type ScreenshotRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	FullPage  bool      `json:"full_page,omitempty"`
	Annotated bool      `json:"annotated,omitempty"`
	Path      string    `json:"path,omitempty"`
}

type PDFRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	Path      string    `json:"path,omitempty"`
}

type Artifact struct {
	Path     string `json:"artifact_path"`
	MimeType string `json:"mime_type,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type EvalRequest struct {
	SessionID SessionID      `json:"session_id"`
	TabID     TabID          `json:"tab_id,omitempty"`
	Script    string         `json:"script"`
	Args      map[string]any `json:"args,omitempty"`
	FrameID   FrameID        `json:"frame_id,omitempty"`
}

type EvalResult struct {
	JSON                     any    `json:"json,omitempty"`
	Error                    string `json:"error,omitempty"`
	NetworkIsolation         bool   `json:"network_isolation"`
	NetworkIsolationAdvisory string `json:"network_isolation_advisory,omitempty"`
}

type WaitRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	Condition string    `json:"condition"`
	TimeoutMS int       `json:"timeout_ms,omitempty"`
}

type WaitResult struct {
	Matched   bool `json:"matched"`
	ElapsedMS int  `json:"elapsed_ms,omitempty"`
}

type TabsRequest struct {
	SessionID SessionID `json:"session_id"`
	Operation string    `json:"operation,omitempty"`
	TabID     TabID     `json:"tab_id,omitempty"`
	URL       string    `json:"url,omitempty"`
}

type TabInfo struct {
	ID     TabID  `json:"tab_id"`
	URL    string `json:"url,omitempty"`
	Title  string `json:"title,omitempty"`
	Active bool   `json:"active,omitempty"`
}

type TabsResult struct {
	Tabs        []TabInfo `json:"tabs"`
	ActiveTabID TabID     `json:"active_tab_id,omitempty"`
}

type MediaSeekRequest struct {
	SessionID SessionID `json:"session_id"`
	TabID     TabID     `json:"tab_id,omitempty"`
	Target    Target    `json:"target,omitempty"`
	Seconds   float64   `json:"seconds"`
	Strategy  string    `json:"strategy,omitempty"`
	TimeoutMS int       `json:"timeout_ms,omitempty"`
}

type MediaSeekResult struct {
	CurrentTime  float64 `json:"current_time"`
	Verified     bool    `json:"verified"`
	StrategyUsed string  `json:"strategy_used,omitempty"`
}

type LessonEvent struct {
	SessionID SessionID      `json:"session_id,omitempty"`
	Kind      string         `json:"kind"`
	Nonce     string         `json:"nonce,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}
