package browser

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Policy struct {
	Enabled                            bool     `json:"enabled"`
	Driver                             string   `json:"driver,omitempty"`
	MaxSessions                        int      `json:"max_sessions,omitempty"`
	SessionTTLSeconds                  int      `json:"session_ttl_seconds,omitempty"`
	AllowedDomains                     []string `json:"allowed_domains,omitempty"`
	AllowFileRoots                     []string `json:"allow_file_roots,omitempty"`
	ArtifactRoot                       string   `json:"artifact_root,omitempty"`
	AllowEval                          bool     `json:"allow_eval,omitempty"`
	AllowUploads                       bool     `json:"allow_uploads,omitempty"`
	AllowDownloads                     bool     `json:"allow_downloads,omitempty"`
	AllowNonLoopbackCDP                bool     `json:"allow_non_loopback_cdp,omitempty"`
	IncludeEvalConsole                 bool     `json:"include_eval_console,omitempty"`
	LessonWrapperStrictExternalScripts bool     `json:"lesson_wrapper_strict_external_scripts,omitempty"`
	SnapshotMaxDepth                   int      `json:"snapshot_max_depth,omitempty"`
	SnapshotMaxOutputByte              int      `json:"snapshot_max_output_bytes,omitempty"`
}

func (p Policy) WithDefaults() Policy {
	if p.MaxSessions <= 0 {
		p.MaxSessions = 2
	}
	if p.SessionTTLSeconds <= 0 {
		p.SessionTTLSeconds = 7200
	}
	if p.Driver == "" {
		p.Driver = "fake"
	}
	if p.SnapshotMaxDepth <= 0 {
		p.SnapshotMaxDepth = 8
	}
	if p.SnapshotMaxOutputByte <= 0 {
		p.SnapshotMaxOutputByte = 65536
	}
	return p
}

func (p Policy) TTL() time.Duration {
	return time.Duration(p.WithDefaults().SessionTTLSeconds) * time.Second
}

var sessionNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$`)

func validSessionName(name string) bool {
	return name == "" || sessionNamePattern.MatchString(name)
}

func (p Policy) checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return Invalid("url must be absolute")
	}
	switch u.Scheme {
	case "http", "https":
		return p.checkDomain(u.Hostname())
	case "file":
		path, err := p.checkFileURL(u)
		if err != nil {
			return err
		}
		return p.checkLocalWrapper(path)
	default:
		return PolicyDenied("scheme_not_allowed", "browser navigation scheme is not allowed")
	}
}

func (p Policy) checkDomain(host string) error {
	if len(p.AllowedDomains) == 0 {
		return nil
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range p.AllowedDomains {
		allowed = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(allowed, ".")))
		if allowed == "" {
			continue
		}
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return PolicyDenied("domain_not_allowed", "browser navigation domain is not allowed")
}

func (p Policy) checkFileURL(u *url.URL) (string, error) {
	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return "", Invalid("file path is invalid")
	}
	if path == "" {
		path = u.Opaque
	}
	if err := p.checkFilePath(path); err != nil {
		return "", err
	}
	return path, nil
}

func (p Policy) checkFilePath(path string) error {
	if len(p.AllowFileRoots) == 0 {
		return PolicyDenied("file_root_not_allowed", "file access is disabled")
	}
	abs, err := canonicalExistingAncestor(path)
	if err != nil {
		return Invalid("file path is invalid")
	}
	for _, root := range p.AllowFileRoots {
		if root == "" {
			continue
		}
		rabs, err := canonicalExistingAncestor(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rabs, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return PolicyDenied("file_root_not_allowed", "file path is outside allowed roots")
}

func (p Policy) checkLocalWrapper(path string) error {
	if !strings.EqualFold(filepath.Ext(path), ".html") && !strings.EqualFold(filepath.Ext(path), ".htm") {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	for _, match := range scriptSrcRE.FindAllStringSubmatch(string(data), -1) {
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		u, err := url.Parse(src)
		if err == nil && u.IsAbs() && u.Scheme != "file" {
			return PolicyDenied("external_script_not_allowed", fmt.Sprintf("trusted local wrapper external script is not allowed: %s", u.Hostname()))
		}
	}
	return nil
}

var scriptSrcRE = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc=["']([^"']+)["']`)

func (p Policy) resolveArtifactPath(path string) (string, error) {
	if p.ArtifactRoot == "" {
		return "", nil
	}
	if path == "" {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return "", PolicyDenied("artifact_path_not_allowed", "artifact path must be relative to artifact root")
	}
	root, err := filepath.Abs(filepath.Clean(p.ArtifactRoot))
	if err != nil {
		return "", Invalid("artifact root is invalid")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	resolved := filepath.Join(root, filepath.Clean(path))
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", PolicyDenied("artifact_path_not_allowed", "artifact path is outside artifact root")
	}
	return resolved, nil
}

func canonicalExistingAncestor(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := abs
	var missing []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, missing[i])
	}
	return filepath.Clean(resolved), nil
}

func (p Policy) checkEndpoint(raw string) error {
	if p.AllowNonLoopbackCDP {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Invalid("endpoint must be an absolute loopback URL")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	return PolicyDenied("endpoint_not_loopback", "browser adapter endpoint must be loopback")
}

func RedactSensitive(s string) string {
	for _, key := range []string{"authorization", "cookie", "localStorage", "sessionStorage"} {
		s = strings.ReplaceAll(s, key, "[redacted]")
	}
	return s
}
