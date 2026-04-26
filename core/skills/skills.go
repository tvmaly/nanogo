// Package skills provides frontmatter parsing, skill discovery, and dispatch.
package skills

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name, Description, Kind, Model string
	Tools, Args                    []string
	Triggers                       map[string]any
	Frontmatter                    map[string]any
	Body, Path                     string
}

type TriggerSource string

const (
	SourceCLI       TriggerSource = "cli"
	SourceREST      TriggerSource = "rest"
	SourceCron      TriggerSource = "cron"
	SourceLLM       TriggerSource = "llm"
	SourceHeartbeat TriggerSource = "heartbeat"
)

type Trigger struct {
	Skill   string
	Source  TriggerSource
	Args    map[string]any
	Session string
}

type Dispatcher interface {
	Fire(ctx context.Context, t Trigger) error
}

type AgentRunner interface {
	RunSkill(ctx context.Context, opts RunSkillOpts) (string, error)
}

type RunSkillOpts struct {
	SystemNote, UserMsg, SkillName, Model string
	Tools                                 []string
	Session                               string
}

func ParseFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseBytes(path, data)
}

func parseBytes(path string, data []byte) (*Skill, error) {
	s := &Skill{Path: path}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		s.Name = strings.TrimSuffix(filepath.Base(path), ".md")
		s.Kind, s.Body, s.Frontmatter = "skill", content, map[string]any{}
		return s, nil
	}
	rest := content[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("skills: unclosed frontmatter in %s", path)
	}
	body := strings.TrimPrefix(rest[idx+4:], "\n")
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(rest[:idx]), &fm); err != nil {
		return nil, fmt.Errorf("skills: malformed YAML in %s: %w", path, err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	s.Frontmatter, s.Body = fm, body
	s.Name, _ = fm["name"].(string)
	if s.Name == "" {
		s.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	s.Description, _ = fm["description"].(string)
	if k, _ := fm["kind"].(string); k != "" {
		s.Kind = k
	} else {
		s.Kind = "skill"
	}
	s.Model, _ = fm["model"].(string)
	s.Tools = strSlice(fm["tools"])
	s.Args = strSlice(fm["args"])
	s.Triggers, _ = fm["triggers"].(map[string]any)
	return s, nil
}

func strSlice(v any) []string {
	sl, _ := v.([]any)
	var out []string
	for _, item := range sl {
		if s, _ := item.(string); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Discover scans dir for .md files with frontmatter, returning all valid skills.
func Discover(dir string, validRoutes []string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	routeSet := make(map[string]bool, len(validRoutes))
	for _, r := range validRoutes {
		routeSet[r] = true
	}
	names := map[string]string{}
	var out []*Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(string(data), "---\n") {
			continue
		}
		sk, err := parseBytes(path, data)
		if err != nil {
			return nil, err
		}
		if prev, ok := names[sk.Name]; ok {
			return nil, fmt.Errorf("skills: duplicate name %q in %s and %s", sk.Name, prev, path)
		}
		names[sk.Name] = path
		if sk.Model != "" && validRoutes != nil && !routeSet[sk.Model] {
			return nil, fmt.Errorf("skills: skill %q references unknown route %q (available: %s)",
				sk.Name, sk.Model, strings.Join(validRoutes, ", "))
		}
		out = append(out, sk)
	}
	return out, nil
}

// Substitute performs {{var}} substitution. Missing keys resolve to "".
func Substitute(body string, args map[string]any) (string, error) {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	fm := template.FuncMap{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		key, val := m[1], ""
		if v, ok := args[key]; ok {
			val = fmt.Sprintf("%v", v)
		}
		fm[key] = func() string { return val }
	}
	tmpl, err := template.New("").Funcs(fm).Parse(body)
	if err != nil {
		return "", fmt.Errorf("skills: template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return "", fmt.Errorf("skills: template exec: %w", err)
	}
	return buf.String(), nil
}

type Loader struct {
	skills []*Skill
	byName map[string]*Skill
}

func NewLoader(skills []*Skill) *Loader {
	m := make(map[string]*Skill, len(skills))
	for _, s := range skills {
		m[s.Name] = s
	}
	return &Loader{skills: skills, byName: m}
}

func (l *Loader) All() []*Skill                     { return l.skills }
func (l *Loader) Lookup(name string) (*Skill, bool) { return l.byName[name], l.byName[name] != nil }

func (l *Loader) UserFacing() []*Skill {
	var out []*Skill
	for _, s := range l.skills {
		if s.Kind != "subagent" {
			out = append(out, s)
		}
	}
	return out
}

type dispatcher struct {
	loader *Loader
	runner AgentRunner
}

func NewDispatcher(loader *Loader, runner AgentRunner) Dispatcher {
	return &dispatcher{loader: loader, runner: runner}
}

func (d *dispatcher) Fire(ctx context.Context, t Trigger) error {
	sk, ok := d.loader.Lookup(t.Skill)
	if !ok {
		return fmt.Errorf("skills: unknown skill %q", t.Skill)
	}
	args := t.Args
	if args == nil {
		args = map[string]any{}
	}
	var missing []string
	for _, a := range sk.Args {
		if _, ok := args[a]; !ok {
			missing = append(missing, a)
		}
	}
	body, err := Substitute(sk.Body, args)
	if err != nil {
		return err
	}
	var note string
	if len(missing) > 0 {
		note = "The following required arguments are missing: " + strings.Join(missing, ", ") +
			". Use the ask_user tool to gather each missing value before proceeding."
	}
	_, err = d.runner.RunSkill(ctx, RunSkillOpts{
		SystemNote: note, UserMsg: body, SkillName: sk.Name,
		Model: sk.Model, Tools: sk.Tools, Session: t.Session,
	})
	return err
}
