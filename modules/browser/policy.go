package browser

import (
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Policy struct {
	Enabled               bool     `json:"enabled"`
	Driver                string   `json:"driver,omitempty"`
	MaxSessions           int      `json:"max_sessions,omitempty"`
	SessionTTLSeconds     int      `json:"session_ttl_seconds,omitempty"`
	AllowedDomains        []string `json:"allowed_domains,omitempty"`
	AllowFileRoots        []string `json:"allow_file_roots,omitempty"`
	ArtifactRoot          string   `json:"artifact_root,omitempty"`
	AllowEval             bool     `json:"allow_eval,omitempty"`
	AllowUploads          bool     `json:"allow_uploads,omitempty"`
	AllowDownloads        bool     `json:"allow_downloads,omitempty"`
	AllowNonLoopbackCDP   bool     `json:"allow_non_loopback_cdp,omitempty"`
	IncludeEvalConsole    bool     `json:"include_eval_console,omitempty"`
	SnapshotMaxDepth      int      `json:"snapshot_max_depth,omitempty"`
	SnapshotMaxOutputByte int      `json:"snapshot_max_output_bytes,omitempty"`
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
		return p.checkFileURL(u)
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

func (p Policy) checkFileURL(u *url.URL) error {
	path := u.Path
	if path == "" {
		path = u.Opaque
	}
	return p.checkFilePath(path)
}

func (p Policy) checkFilePath(path string) error {
	if len(p.AllowFileRoots) == 0 {
		return PolicyDenied("file_root_not_allowed", "file access is disabled")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Invalid("file path is invalid")
	}
	for _, root := range p.AllowFileRoots {
		if root == "" {
			continue
		}
		rabs, err := filepath.Abs(root)
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
