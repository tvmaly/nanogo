package fake

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/modules/browser"
)

type Controller struct {
	mu                 sync.Mutex
	nextSess           int
	nextTab            int
	sessions           map[browser.SessionID]*state
	Closed             []browser.SessionID
	StartDelay         time.Duration
	LastScreenshotPath string
}

type state struct {
	session browser.Session
	url     string
	title   string
	version int64
	inputs  map[browser.Ref]string
	selects map[browser.Ref]string
	scroll  int
	media   float64
}

func New() *Controller {
	return &Controller{sessions: map[browser.SessionID]*state{}}
}

func (c *Controller) Health(context.Context) (browser.Health, error) {
	return browser.Health{OK: true, Driver: "fake", Version: "0.0.0", Capabilities: []string{"snapshot", "click", "media_seek"}}, nil
}

func (c *Controller) Start(ctx context.Context, req browser.StartRequest) (browser.Session, error) {
	if c.StartDelay > 0 {
		select {
		case <-time.After(c.StartDelay):
		case <-ctx.Done():
			return browser.Session{}, ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSess++
	c.nextTab++
	id := browser.SessionID(fmt.Sprintf("fake-session-%d", c.nextSess))
	tab := browser.TabID(fmt.Sprintf("fake-tab-%d", c.nextTab))
	sess := browser.Session{
		ID:           id,
		Name:         req.SessionName,
		ActiveTabID:  tab,
		Headed:       req.Headed,
		Capabilities: []string{"snapshot", "click", "fill", "text", "screenshot", "tabs", "wait", "media_seek"},
		Metadata:     map[string]string{"driver": "fake"},
		CreatedAt:    time.Now(),
		LastUsedAt:   time.Now(),
	}
	c.sessions[id] = &state{session: sess, title: "Blank", version: 1, inputs: map[browser.Ref]string{}, selects: map[browser.Ref]string{}}
	return sess, nil
}

func (c *Controller) Connect(context.Context, browser.ConnectRequest) (browser.Session, error) {
	return browser.Session{}, browser.E(browser.CodeAdapterUnavailable, "fake connect is not implemented")
}

func (c *Controller) Close(_ context.Context, req browser.CloseRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, req.SessionID)
	c.Closed = append(c.Closed, req.SessionID)
	return nil
}

func (c *Controller) Navigate(_ context.Context, req browser.NavigateRequest) (browser.PageState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.state(req.SessionID)
	if err != nil {
		return browser.PageState{}, err
	}
	st.url = req.URL
	st.title = titleFor(req.URL)
	st.version++
	return browser.PageState{SessionID: req.SessionID, TabID: st.session.ActiveTabID, URL: st.url, Title: st.title, Status: "loaded", Version: st.version}, nil
}

func (c *Controller) Snapshot(_ context.Context, req browser.SnapshotRequest) (browser.Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.state(req.SessionID)
	if err != nil {
		return browser.Snapshot{}, err
	}
	text := "Start lesson\nAnswer\nGrade\nNext"
	nodes := []browser.SnapshotNode{
		{Ref: ref(st.version, 1), AdapterRef: "fake://button/start", Role: "button", Label: "Start lesson"},
		{Ref: ref(st.version, 2), AdapterRef: "fake://input/answer", Role: "textbox", Label: "Answer"},
		{Ref: ref(st.version, 3), AdapterRef: "fake://select/grade", Role: "combobox", Label: "Grade"},
		{Ref: ref(st.version, 4), AdapterRef: "fake://button/next", Role: "button", Label: "Next"},
	}
	truncated := false
	reason := ""
	if req.MaxOutputBytes > 0 && len(text) > req.MaxOutputBytes {
		text = text[:req.MaxOutputBytes]
		truncated = true
		reason = "max_output_bytes"
	}
	var frames []browser.FrameInfo
	if req.IncludeIFrames {
		frames = []browser.FrameInfo{{ID: "frame-1", URL: "https://external.example.test/embed", CrossOrigin: true}}
	}
	return browser.Snapshot{
		SessionID: req.SessionID, TabID: st.session.ActiveTabID, SnapshotID: fmt.Sprintf("snap-%d", st.version),
		Version: st.version, Text: text, Nodes: nodes, Frames: frames, SnapshotTruncated: truncated, TruncationReason: reason,
	}, nil
}

func (c *Controller) Text(_ context.Context, req browser.TextRequest) (browser.TextResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.state(req.SessionID); err != nil {
		return browser.TextResult{}, err
	}
	return browser.TextResult{Text: "Start lesson\nAnswer\nGrade\nNext", Source: "fake"}, nil
}

func (c *Controller) Screenshot(_ context.Context, req browser.ScreenshotRequest) (browser.Artifact, error) {
	if _, err := c.state(req.SessionID); err != nil {
		return browser.Artifact{}, err
	}
	path := req.Path
	if path == "" {
		path = filepath.Join("artifacts", "browser", "fake.png")
	}
	c.LastScreenshotPath = path
	return browser.Artifact{Path: path, MimeType: "image/png", Width: 800, Height: 600}, nil
}

func (c *Controller) PDF(_ context.Context, req browser.PDFRequest) (browser.Artifact, error) {
	if _, err := c.state(req.SessionID); err != nil {
		return browser.Artifact{}, err
	}
	path := req.Path
	if path == "" {
		path = filepath.Join("artifacts", "browser", "fake.pdf")
	}
	return browser.Artifact{Path: path, MimeType: "application/pdf"}, nil
}

func (c *Controller) Act(_ context.Context, req browser.ActionRequest) (browser.ActionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.state(req.SessionID)
	if err != nil {
		return browser.ActionResult{}, err
	}
	if req.Target.Ref != "" && !strings.HasPrefix(string(req.Target.Ref), fmt.Sprintf("ref://v%d/", st.version)) {
		return browser.ActionResult{}, browser.E(browser.CodeStaleRef, "request a fresh snapshot before retrying")
	}
	switch req.Kind {
	case browser.ActionFill:
		st.inputs[req.Target.Ref] = req.Value
	case browser.ActionSelect:
		st.selects[req.Target.Ref] = req.Value
	case browser.ActionScroll:
		st.scroll++
	}
	return browser.ActionResult{Success: true}, nil
}

func (c *Controller) Eval(context.Context, browser.EvalRequest) (browser.EvalResult, error) {
	return browser.EvalResult{JSON: map[string]any{"ok": true}}, nil
}

func (c *Controller) Wait(context.Context, browser.WaitRequest) (browser.WaitResult, error) {
	return browser.WaitResult{Matched: true, ElapsedMS: 100}, nil
}

func (c *Controller) Tabs(_ context.Context, req browser.TabsRequest) (browser.TabsResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.state(req.SessionID)
	if err != nil {
		return browser.TabsResult{}, err
	}
	return browser.TabsResult{ActiveTabID: st.session.ActiveTabID, Tabs: []browser.TabInfo{{ID: st.session.ActiveTabID, URL: st.url, Title: st.title, Active: true}}}, nil
}

func (c *Controller) MediaSeek(_ context.Context, req browser.MediaSeekRequest) (browser.MediaSeekResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, err := c.state(req.SessionID)
	if err != nil {
		return browser.MediaSeekResult{}, err
	}
	st.media = req.Seconds
	strategy := req.Strategy
	if strategy == "auto" {
		strategy = "html5_video"
	}
	return browser.MediaSeekResult{CurrentTime: st.media, Verified: true, StrategyUsed: strategy}, nil
}

func (c *Controller) InputValue(id browser.SessionID, ref browser.Ref) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[id].inputs[ref]
}

func (c *Controller) SelectedValue(id browser.SessionID, ref browser.Ref) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[id].selects[ref]
}

func (c *Controller) state(id browser.SessionID) (*state, error) {
	st := c.sessions[id]
	if st == nil {
		return nil, browser.E(browser.CodeNotFound, "browser session not found")
	}
	return st, nil
}

func ref(version int64, n int) browser.Ref {
	return browser.Ref(fmt.Sprintf("ref://v%d/e%d", version, n))
}

func titleFor(raw string) string {
	if strings.Contains(raw, "example") {
		return "Example lesson"
	}
	return "Lesson"
}
