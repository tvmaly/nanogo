// Package progressive implements manifest-backed tool progressive disclosure.
package progressive

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	coretools "github.com/tvmaly/nanogo/core/tools"
)

type Manifest struct {
	Version        int            `json:"version"`
	DefaultVisible []string       `json:"default_visible"`
	Tools          []ManifestTool `json:"tools"`
	Groups         []Group        `json:"groups,omitempty"`
}

type ManifestTool struct {
	Name        string         `json:"name"`
	Source      string         `json:"source,omitempty"`
	Group       string         `json:"group,omitempty"`
	Description string         `json:"description,omitempty"`
	Reveal      RevealPolicy   `json:"reveal,omitempty"`
	Safety      Safety         `json:"safety,omitempty"`
	Output      OutputContract `json:"output,omitempty"`
	DataAccess  DataAccess     `json:"data_access,omitempty"`
	Examples    []Example      `json:"examples,omitempty"`
}

type RevealPolicy struct {
	Mode           string   `json:"mode,omitempty"`
	Requires       []string `json:"requires,omitempty"`
	ReasonRequired bool     `json:"reason_required,omitempty"`
}

type Safety struct {
	RequiresApproval bool `json:"requires_approval,omitempty"`
	Destructive      bool `json:"destructive,omitempty"`
	Network          bool `json:"network,omitempty"`
	Filesystem       bool `json:"filesystem,omitempty"`
	ChildData        bool `json:"child_data,omitempty"`
}

type OutputContract struct {
	Mode           string `json:"mode,omitempty"`
	MaxOutputBytes int    `json:"max_output_bytes,omitempty"`
	MaxItems       int    `json:"max_items,omitempty"`
}

type DataAccess struct {
	Mode      string `json:"mode,omitempty"`
	Freshness string `json:"freshness,omitempty"`
	SyncTool  string `json:"sync_tool,omitempty"`
}

type Example struct {
	Name string          `json:"name,omitempty"`
	Args json.RawMessage `json:"args,omitempty"`
}

type Group struct {
	Name         string   `json:"name"`
	VisibleAfter []string `json:"visible_after,omitempty"`
	MaxTools     int      `json:"max_tools,omitempty"`
}

type Source struct {
	child    coretools.Source
	manifest Manifest
	byName   map[string]ManifestTool

	mu       sync.RWMutex
	revealed map[string]map[string]bool
}

func NewSource(child coretools.Source, manifest Manifest) (*Source, error) {
	if child == nil {
		return nil, fmt.Errorf("progressive: child source is required")
	}
	src := &Source{
		child:    child,
		manifest: manifest,
		byName:   make(map[string]ManifestTool, len(manifest.Tools)),
		revealed: make(map[string]map[string]bool),
	}
	if err := src.validate(context.Background()); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Source) Tools(ctx context.Context, turn coretools.TurnInfo) ([]coretools.Tool, error) {
	child, err := s.child.Tools(ctx, turn)
	if err != nil {
		return nil, err
	}
	childByName := mapTools(child)
	visible := map[string]bool{}
	for _, name := range s.manifest.DefaultVisible {
		visible[name] = true
	}
	s.mu.RLock()
	for name := range s.revealed[turn.Session] {
		visible[name] = true
	}
	s.mu.RUnlock()

	out := []coretools.Tool{
		&listTool{source: s, session: turn.Session},
		&helpTool{source: s},
		&revealTool{source: s, session: turn.Session},
	}
	for name := range visible {
		if tool, ok := childByName[name]; ok {
			out = append(out, tool)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (s *Source) validate(ctx context.Context) error {
	child, err := s.child.Tools(ctx, coretools.TurnInfo{Session: "_validate"})
	if err != nil {
		return err
	}
	childByName := mapTools(child)
	groups := map[string]bool{}
	for _, g := range s.manifest.Groups {
		if g.Name == "" {
			return fmt.Errorf("progressive: group name is required")
		}
		if groups[g.Name] {
			return fmt.Errorf("progressive: duplicate group %q", g.Name)
		}
		groups[g.Name] = true
	}
	for _, mt := range s.manifest.Tools {
		if mt.Name == "" {
			return fmt.Errorf("progressive: tool name is required")
		}
		if _, ok := childByName[mt.Name]; !ok {
			return fmt.Errorf("progressive: manifest tool %q not found in child source", mt.Name)
		}
		if _, exists := s.byName[mt.Name]; exists {
			return fmt.Errorf("progressive: duplicate tool %q", mt.Name)
		}
		if mt.Group != "" && len(groups) > 0 && !groups[mt.Group] {
			return fmt.Errorf("progressive: unknown group %q", mt.Group)
		}
		if mode := mt.Reveal.Mode; mode != "" && mode != "explicit" && mode != "auto" {
			return fmt.Errorf("progressive: invalid reveal mode %q", mode)
		}
		s.byName[mt.Name] = mt
	}
	for _, name := range s.manifest.DefaultVisible {
		if _, ok := childByName[name]; !ok {
			return fmt.Errorf("progressive: default visible tool %q not found", name)
		}
	}
	return s.validateRevealGraph()
}

func (s *Source) validateRevealGraph() error {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("progressive: reveal dependency cycle at %q", name)
		}
		mt, ok := s.byName[name]
		if !ok {
			return fmt.Errorf("progressive: unknown reveal dependency %q", name)
		}
		visiting[name] = true
		for _, dep := range mt.Reveal.Requires {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for name := range s.byName {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) reveal(session, name, reason string) error {
	mt, ok := s.byName[name]
	if !ok {
		return fmt.Errorf("progressive: tool %q not registered", name)
	}
	if mt.Reveal.ReasonRequired && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("progressive: reveal reason required for %q", name)
	}
	if mt.Safety.Destructive {
		return fmt.Errorf("progressive: tool %q is destructive and requires approval policy", name)
	}
	if mt.Safety.RequiresApproval || mt.Safety.Network || mt.Safety.Filesystem || mt.Safety.ChildData {
		if mt.Reveal.Mode != "explicit" || !mt.Reveal.ReasonRequired {
			return fmt.Errorf("progressive: tool %q safety policy is under-specified", name)
		}
	}
	s.mu.RLock()
	revealed := s.revealed[session]
	for _, dep := range mt.Reveal.Requires {
		if !revealed[dep] && !contains(s.manifest.DefaultVisible, dep) {
			s.mu.RUnlock()
			return fmt.Errorf("progressive: reveal %q requires %q", name, dep)
		}
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revealed[session] == nil {
		s.revealed[session] = map[string]bool{}
	}
	s.revealed[session][name] = true
	return nil
}

func (s *Source) list(session string) []map[string]any {
	s.mu.RLock()
	revealed := s.revealed[session]
	s.mu.RUnlock()
	rows := make([]map[string]any, 0, len(s.byName)+3)
	defaults := map[string]bool{}
	for _, name := range s.manifest.DefaultVisible {
		defaults[name] = true
	}
	for name, mt := range s.byName {
		status := "hidden"
		if defaults[name] || revealed[name] {
			status = "visible"
		} else if mt.Reveal.Mode != "" {
			status = "revealable"
		}
		rows = append(rows, map[string]any{
			"name":        name,
			"group":       mt.Group,
			"description": mt.Description,
			"status":      status,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
	return rows
}

func (s *Source) help(name string) (ManifestTool, error) {
	mt, ok := s.byName[name]
	if !ok {
		return ManifestTool{}, fmt.Errorf("progressive: tool %q not found", name)
	}
	return mt, nil
}

func mapTools(list []coretools.Tool) map[string]coretools.Tool {
	out := make(map[string]coretools.Tool, len(list))
	for _, tool := range list {
		out[tool.Name()] = tool
	}
	return out
}

func contains(list []string, name string) bool {
	for _, v := range list {
		if v == name {
			return true
		}
	}
	return false
}

type listTool struct {
	source  *Source
	session string
}

func (*listTool) Name() string { return "tool_list" }
func (*listTool) Schema() json.RawMessage {
	return schema("tool_list", "List visible and revealable tools.", map[string]any{})
}
func (t *listTool) Call(context.Context, json.RawMessage) (string, error) {
	b, _ := json.Marshal(map[string]any{"tools": t.source.list(t.session)})
	return string(b), nil
}

type helpTool struct{ source *Source }

func (*helpTool) Name() string { return "tool_help" }
func (*helpTool) Schema() json.RawMessage {
	return schema("tool_help", "Show bounded help for one tool.", map[string]any{
		"name": map[string]any{"type": "string"},
	})
}
func (t *helpTool) Call(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	mt, err := t.source.help(in.Name)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{
		"name":        mt.Name,
		"group":       mt.Group,
		"description": mt.Description,
		"safety":      mt.Safety,
		"output":      mt.Output,
		"data_access": mt.DataAccess,
		"reveal":      mt.Reveal,
	})
	return string(b), nil
}

type revealTool struct {
	source  *Source
	session string
}

func (*revealTool) Name() string { return "tool_reveal" }
func (*revealTool) Schema() json.RawMessage {
	return schema("tool_reveal", "Reveal a hidden tool for this session.", map[string]any{
		"name":   map[string]any{"type": "string"},
		"reason": map[string]any{"type": "string"},
	})
}
func (t *revealTool) Call(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	if err := t.source.reveal(t.session, in.Name, in.Reason); err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"revealed": in.Name})
	return string(b), nil
}

func schema(name, desc string, properties map[string]any) json.RawMessage {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	sort.Strings(required)
	b, _ := json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": desc,
			"parameters": map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	})
	return b
}
