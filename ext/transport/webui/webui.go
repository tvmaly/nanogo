// Package webui implements the web tutor UI transport extension.
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/modules/transport"
)

func init() {
	transport.Register("webui", func(cfg json.RawMessage, bus event.Bus, app transport.App) (transport.Transport, error) {
		var c Config
		if err := json.Unmarshal(cfg, &c); err != nil {
			return nil, err
		}
		if c.Addr == "" {
			c.Addr = ":8090"
		}
		return New(c), nil
	})
}

// Config holds web UI configuration.
type Config struct {
	Addr             string   `json:"addr"`
	Lessons          []Lesson `json:"lessons,omitempty"`
	InsecureSkipAuth bool     `json:"insecure_skip_auth,omitempty"`
}

// Lesson is a single lesson definition.
type Lesson struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}

// Block is one content block within a lesson.
type Block struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // prose|video|quiz|interactive|manim
	Content   string `json:"content,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	QuizRef   string `json:"quiz_ref,omitempty"`
	AssetPath string `json:"asset_path,omitempty"`
	Caption   string `json:"caption,omitempty"`
}

// allowedVideoHosts is the allowlist for remote video embeds.
var allowedVideoHosts = map[string]bool{
	"www.youtube.com":  true,
	"youtube.com":      true,
	"youtu.be":         true,
	"vimeo.com":        true,
	"player.vimeo.com": true,
}

// ValidateVideoURL returns an error if the URL host is not on the allowlist.
func ValidateVideoURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid video URL: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if !allowedVideoHosts[host] {
		return fmt.Errorf("video host %q is not on the allowlist", host)
	}
	return nil
}

// ValidateInteractiveAsset returns an error if the path is a remote URL.
func ValidateInteractiveAsset(path string) error {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Errorf("interactive asset must be a same-origin path, not a remote URL: %q", path)
	}
	return nil
}

// WebUI is the HTTP server for the tutor web interface.
type WebUI struct {
	cfg    Config
	mux    *http.ServeMux
	server *http.Server
	byID   map[string]Lesson
}

// New constructs a WebUI server.
func New(cfg Config) *WebUI {
	w := &WebUI{cfg: cfg, byID: make(map[string]Lesson)}
	for _, l := range cfg.Lessons {
		w.byID[l.ID] = l
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", w.handleHealthz)
	mux.HandleFunc("GET /student", w.handleStudentShell)
	mux.HandleFunc("GET /parent", w.handleParentShell)
	// Lesson page — auth required per role
	mux.HandleFunc("GET /student/lesson/{id}", w.requireRole("student", w.handleStudentLesson))
	// Parent-only routes
	mux.HandleFunc("GET /parent/dashboard", w.requireRole("parent", w.handleParentDashboard))
	mux.HandleFunc("GET /parent/reports", w.requireRole("parent", w.handleParentReports))
	mux.HandleFunc("GET /parent/lessons/new", w.requireRole("parent", w.handleParentLessonNew))
	w.mux = mux
	w.server = &http.Server{Addr: cfg.Addr, Handler: mux}
	return w
}

// ServeHTTP implements http.Handler so the WebUI can be used with httptest.
func (w *WebUI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.mux.ServeHTTP(rw, r)
}

// Name returns the transport driver name.
func (w *WebUI) Name() string { return "webui" }

// Start begins listening.
func (w *WebUI) Start(ctx context.Context, app transport.App) error {
	return w.server.ListenAndServe()
}

// Stop gracefully shuts down.
func (w *WebUI) Stop(ctx context.Context) error {
	return w.server.Shutdown(ctx)
}

func (w *WebUI) handleHealthz(rw http.ResponseWriter, r *http.Request) {
	rw.Write([]byte("ok"))
}

var studentShellTmpl = template.Must(template.New("student").Parse(`<!DOCTYPE html>
<html><head><title>Tutor — Student</title>
<script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head><body>
<h1>Student Portal</h1>
<nav><a href="/student">Home</a></nav>
<div id="content"></div>
</body></html>`))

var parentShellTmpl = template.Must(template.New("parent").Parse(`<!DOCTYPE html>
<html><head><title>Tutor — Parent</title>
<script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head><body>
<h1>Parent Portal</h1>
<nav>
  <a href="/parent/dashboard">Dashboard</a> |
  <a href="/parent/reports">Reports</a> |
  <a href="/parent/lessons/new">New Lesson</a>
</nav>
<div id="content"></div>
</body></html>`))

var lessonTmpl = template.Must(template.New("lesson").Parse(`<!DOCTYPE html>
<html><head><title>{{.Title}}</title>
<script src="https://unpkg.com/htmx.org@1.9.10"></script>
</head><body>
<h1>{{.Title}}</h1>
{{range .Blocks}}
<section id="{{.ID}}" class="block block-{{.Kind}}">
{{if eq .Kind "prose"}}<div class="prose">{{.Content}}</div>
{{else if eq .Kind "video"}}<iframe src="{{.VideoURL}}" sandbox="allow-scripts allow-same-origin" loading="lazy" allowfullscreen></iframe>
{{else if eq .Kind "quiz"}}<div class="quiz" data-ref="{{.QuizRef}}">Quiz: {{.QuizRef}}</div>
{{else if eq .Kind "interactive"}}<div class="interactive" data-src="{{.AssetPath}}">Interactive: {{.AssetPath}}</div>
{{else if eq .Kind "manim"}}<div class="manim"><video src="{{.AssetPath}}" controls></video><p>{{.Caption}}</p></div>
{{end}}
</section>
{{end}}
</body></html>`))

func (w *WebUI) handleStudentShell(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	studentShellTmpl.Execute(rw, nil)
}

func (w *WebUI) handleParentShell(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	parentShellTmpl.Execute(rw, nil)
}

func (w *WebUI) handleStudentLesson(rw http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lesson, ok := w.byID[id]
	if !ok {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	lessonTmpl.Execute(rw, lesson)
}

func (w *WebUI) handleParentDashboard(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write([]byte(`<!DOCTYPE html><html><body><h1>Parent Dashboard</h1></body></html>`))
}

func (w *WebUI) handleParentReports(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write([]byte(`<!DOCTYPE html><html><body><h1>Reports</h1></body></html>`))
}

func (w *WebUI) handleParentLessonNew(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write([]byte(`<!DOCTYPE html><html><body><h1>New Lesson</h1></body></html>`))
}

// requireRole wraps a handler with role-based access control.
// Role is determined by cookie "role" matching the required value.
// InsecureSkipAuth bypasses this for tests.
func (w *WebUI) requireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if w.cfg.InsecureSkipAuth {
			next(rw, r)
			return
		}
		c, err := r.Cookie("role")
		if err != nil || c.Value != role {
			http.Error(rw, "Forbidden", http.StatusForbidden)
			return
		}
		next(rw, r)
	}
}
