package agentbrowser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tvmaly/nanogo/modules/browser"
)

const minVersion = "0.27.0"

type Runner interface {
	Run(context.Context, []string) ([]byte, []byte, error)
}

type execRunner struct {
	bin string
}

func NewExecRunner(bin string) Runner {
	if bin == "" {
		bin = "agent-browser"
	}
	return execRunner{bin: bin}
}

func (r execRunner) Run(ctx context.Context, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type Adapter struct {
	runner Runner
	mu     sync.Mutex
	refs   map[string]string
}

func New(runner Runner) *Adapter {
	if runner == nil {
		runner = NewExecRunner("agent-browser")
	}
	return &Adapter{runner: runner, refs: map[string]string{}}
}

func (a *Adapter) Health(ctx context.Context) (browser.Health, error) {
	out, errout, err := a.runner.Run(ctx, []string{"--version"})
	if err != nil {
		return browser.Health{}, browser.E(browser.CodeAdapterUnavailable, strings.TrimSpace(string(errout)))
	}
	version, err := parseVersion(string(out))
	if err != nil {
		return browser.Health{}, browser.E(browser.CodeAdapterUnavailable, err.Error())
	}
	if compareVersion(version, minVersion) < 0 {
		return browser.Health{}, browser.E(browser.CodeUnsupportedVersion, "agent-browser >= 0.27.0 is required")
	}
	return browser.Health{OK: true, Driver: "agent-browser", Version: version, Capabilities: []string{"snapshot", "click", "tabs", "media_seek"}}, nil
}

func (a *Adapter) Start(ctx context.Context, req browser.StartRequest) (browser.Session, error) {
	sessionName := req.SessionName
	if sessionName == "" {
		sessionName = fmt.Sprintf("nanogo-%08x", rand.Uint32())
	}
	args := []string{"open", "--json", "--session", sessionName}
	if req.SessionName != "" {
		args = append(args, "--session-name", req.SessionName)
	}
	if req.Headed {
		args = append(args, "--headed")
	}
	for _, d := range req.AllowedDomains {
		args = append(args, "--allowed-domains", d)
	}
	for range req.FileRoots {
		args = append(args, "--allow-file-access")
		break
	}
	var raw map[string]any
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.Session{}, err
	}
	return browser.Session{
		ID: browser.SessionID(sessionName), Name: req.SessionName, ActiveTabID: "active", Headed: req.Headed,
		Capabilities: []string{"snapshot", "click", "fill", "text", "screenshot", "tabs", "wait", "media_seek"},
		Metadata:     map[string]string{"driver": "agent-browser"},
	}, nil
}

func (a *Adapter) Connect(context.Context, browser.ConnectRequest) (browser.Session, error) {
	return browser.Session{}, browser.E(browser.CodeInvalidRequest, "agent-browser connect is gateway-only and not implemented in v1")
}

func (a *Adapter) Close(ctx context.Context, req browser.CloseRequest) error {
	args := []string{"close", "--json", "--session", string(req.SessionID)}
	_, _, err := a.runner.Run(ctx, args)
	return mapError(err, nil)
}

func (a *Adapter) Navigate(ctx context.Context, req browser.NavigateRequest) (browser.PageState, error) {
	args := []string{"open", req.URL, "--json", "--session", string(req.SessionID)}
	if req.WaitUntil != "" {
		args = append(args, "--wait-until", req.WaitUntil)
	}
	if u, err := url.Parse(req.URL); err == nil && u.Scheme == "file" {
		args = append(args, "--allow-file-access")
	}
	var raw struct {
		TabID   string `json:"tab_id"`
		URL     string `json:"url"`
		Title   string `json:"title"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.PageState{}, err
	}
	if raw.Status == "" {
		raw.Status = "loaded"
	}
	if raw.TabID == "" {
		raw.TabID = "active"
	}
	return browser.PageState{SessionID: req.SessionID, TabID: browser.TabID(raw.TabID), URL: raw.URL, Title: raw.Title, Status: raw.Status, Version: raw.Version}, nil
}

func (a *Adapter) Snapshot(ctx context.Context, req browser.SnapshotRequest) (browser.Snapshot, error) {
	args := []string{"snapshot", "--json", "--session", string(req.SessionID)}
	if req.InteractiveOnly {
		args = append(args, "--interactive-only")
	}
	var raw struct {
		SnapshotID string `json:"snapshot_id"`
		Version    int64  `json:"version"`
		Text       string `json:"text"`
		Snapshot   string `json:"snapshot"`
		Refs       map[string]struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"refs"`
		Nodes []struct {
			Ref   string `json:"ref"`
			Role  string `json:"role"`
			Label string `json:"label"`
			Text  string `json:"text"`
		} `json:"nodes"`
	}
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.Snapshot{}, err
	}
	if raw.Version == 0 {
		raw.Version = 1
	}
	if raw.Text == "" {
		raw.Text = raw.Snapshot
	}
	nodes := make([]browser.SnapshotNode, 0, len(raw.Nodes))
	a.mu.Lock()
	i := 0
	keys := make([]string, 0, len(raw.Refs))
	for key := range raw.Refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ref := raw.Refs[key]
		i++
		adapterRef := "@" + key
		nref := browser.Ref(fmt.Sprintf("ref://v%d/e%d", raw.Version, i))
		a.refs[string(nref)] = adapterRef
		nodes = append(nodes, browser.SnapshotNode{Ref: nref, AdapterRef: adapterRef, Role: ref.Role, Label: ref.Name, Text: ref.Name})
	}
	for _, node := range raw.Nodes {
		i++
		nref := browser.Ref(fmt.Sprintf("ref://v%d/e%d", raw.Version, i))
		a.refs[string(nref)] = node.Ref
		nodes = append(nodes, browser.SnapshotNode{
			Ref: nref, AdapterRef: node.Ref,
			Role: node.Role, Label: node.Label, Text: node.Text,
		})
	}
	a.mu.Unlock()
	return browser.Snapshot{SessionID: req.SessionID, TabID: req.TabID, SnapshotID: raw.SnapshotID, Version: raw.Version, Text: raw.Text, Nodes: nodes}, nil
}

func (a *Adapter) Text(ctx context.Context, req browser.TextRequest) (browser.TextResult, error) {
	var raw browser.TextResult
	if err := a.runDataJSON(ctx, []string{"get", "text", "--json", "--session", string(req.SessionID)}, &raw); err != nil {
		return browser.TextResult{}, err
	}
	return raw, nil
}

func (a *Adapter) Screenshot(ctx context.Context, req browser.ScreenshotRequest) (browser.Artifact, error) {
	args := []string{"screenshot"}
	if req.Annotated {
		args = append(args, "--annotate")
	}
	if req.FullPage {
		args = append(args, "--full")
	}
	if req.Path != "" {
		args = append(args, req.Path)
	}
	args = append(args, "--json", "--session", string(req.SessionID))
	var raw struct {
		browser.Artifact
		Path     string `json:"path"`
		File     string `json:"file"`
		Filename string `json:"filename"`
	}
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.Artifact{}, err
	}
	artifact := raw.Artifact
	if artifact.Path == "" {
		artifact.Path = firstNonEmpty(raw.Path, raw.File, raw.Filename, req.Path)
	}
	return artifact, nil
}

func (a *Adapter) PDF(ctx context.Context, req browser.PDFRequest) (browser.Artifact, error) {
	var raw browser.Artifact
	if err := a.runDataJSON(ctx, []string{"pdf", "--json", "--session", string(req.SessionID)}, &raw); err != nil {
		return browser.Artifact{}, err
	}
	return raw, nil
}

func (a *Adapter) Act(ctx context.Context, req browser.ActionRequest) (browser.ActionResult, error) {
	args := a.actionArgs(req)
	if req.Target.Ref != "" {
		args = append(args, "--ref", string(req.Target.Ref))
	}
	if req.Value != "" {
		args = append(args, "--value", req.Value)
	}
	var raw browser.ActionResult
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.ActionResult{}, err
	}
	return raw, nil
}

func (a *Adapter) Eval(ctx context.Context, req browser.EvalRequest) (browser.EvalResult, error) {
	var raw browser.EvalResult
	if err := a.runDataJSON(ctx, []string{"eval", req.Script, "--json", "--session", string(req.SessionID)}, &raw); err != nil {
		return browser.EvalResult{}, err
	}
	return raw, nil
}

func (a *Adapter) Wait(ctx context.Context, req browser.WaitRequest) (browser.WaitResult, error) {
	var raw browser.WaitResult
	if err := a.runDataJSON(ctx, []string{"wait", req.Condition, "--json", "--session", string(req.SessionID)}, &raw); err != nil {
		return browser.WaitResult{}, err
	}
	return raw, nil
}

func (a *Adapter) Tabs(ctx context.Context, req browser.TabsRequest) (browser.TabsResult, error) {
	var raw browser.TabsResult
	if err := a.runDataJSON(ctx, []string{"tab", "list", "--json", "--session", string(req.SessionID)}, &raw); err != nil {
		return browser.TabsResult{}, err
	}
	return raw, nil
}

func (a *Adapter) MediaSeek(ctx context.Context, req browser.MediaSeekRequest) (browser.MediaSeekResult, error) {
	args := []string{"media", "seek", "--json", "--session", string(req.SessionID), "--seconds", strconv.FormatFloat(req.Seconds, 'f', -1, 64), "--strategy", req.Strategy}
	var raw browser.MediaSeekResult
	if err := a.runDataJSON(ctx, args, &raw); err != nil {
		return browser.MediaSeekResult{}, err
	}
	return raw, nil
}

func (a *Adapter) runJSON(ctx context.Context, args []string, dst any) error {
	out, errout, err := a.runner.Run(ctx, args)
	if err != nil {
		return mapError(err, errout)
	}
	if err := json.Unmarshal(out, dst); err != nil {
		return browser.E(browser.CodeInvalidRequest, "agent-browser returned malformed JSON")
	}
	return nil
}

func (a *Adapter) runDataJSON(ctx context.Context, args []string, dst any) error {
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   any             `json:"error"`
	}
	if err := a.runJSON(ctx, args, &env); err != nil {
		return err
	}
	if !env.Success {
		return browser.E(browser.CodeAdapterUnavailable, fmt.Sprint(env.Error))
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		return browser.E(browser.CodeInvalidRequest, "agent-browser returned malformed data JSON")
	}
	return nil
}

func (a *Adapter) actionArgs(req browser.ActionRequest) []string {
	target := string(req.Target.Ref)
	if target != "" {
		a.mu.Lock()
		if mapped := a.refs[target]; mapped != "" {
			target = mapped
		}
		a.mu.Unlock()
	}
	if target == "" {
		target = req.Target.Selector
	}
	switch req.Kind {
	case browser.ActionClick:
		return []string{"click", target, "--json", "--session", string(req.SessionID)}
	case browser.ActionFill:
		return []string{"fill", target, req.Value, "--json", "--session", string(req.SessionID)}
	case browser.ActionType:
		return []string{"type", target, req.Value, "--json", "--session", string(req.SessionID)}
	case browser.ActionPress:
		return []string{"press", req.Value, "--json", "--session", string(req.SessionID)}
	case browser.ActionHover:
		return []string{"hover", target, "--json", "--session", string(req.SessionID)}
	case browser.ActionSelect:
		return []string{"select", target, req.Value, "--json", "--session", string(req.SessionID)}
	case browser.ActionScroll:
		return []string{"scroll", req.Value, "--json", "--session", string(req.SessionID)}
	default:
		return []string{"click", target, "--json", "--session", string(req.SessionID)}
	}
}

func mapError(err error, stderr []byte) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = err.Error()
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "stale"):
		return browser.E(browser.CodeStaleRef, msg)
	case strings.Contains(lower, "timeout"):
		return browser.E(browser.CodeTimeout, msg)
	case strings.Contains(lower, "not found"):
		return browser.E(browser.CodeNotFound, msg)
	case strings.Contains(lower, "cross-origin"):
		return browser.E(browser.CodeCrossOriginBlocked, msg)
	default:
		return browser.E(browser.CodeAdapterUnavailable, msg)
	}
}

var versionRE = regexp.MustCompile(`(\d+\.\d+\.\d+)([-+][0-9A-Za-z.-]+)?`)

func parseVersion(s string) (string, error) {
	m := versionRE.FindStringSubmatch(s)
	if len(m) != 3 {
		return "", fmt.Errorf("could not parse agent-browser version")
	}
	if strings.HasPrefix(m[2], "-") {
		return m[1] + m[2], nil
	}
	return m[1], nil
}

func compareVersion(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	aPre := strings.Contains(a, "-")
	bPre := strings.Contains(b, "-")
	if aPre && !bPre {
		return -1
	}
	if !aPre && bPre {
		return 1
	}
	return 0
}

func splitVersion(s string) [3]int {
	var out [3]int
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
