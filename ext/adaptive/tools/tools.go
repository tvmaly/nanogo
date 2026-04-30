// Package tools exposes adaptive archive/profile/report operations as agent tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/profile"
	"github.com/tvmaly/nanogo/ext/adaptive/reports"
)

func init() {
	coretools.Register("adaptive", func(cfg json.RawMessage) (coretools.Source, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return NewSource(c)
	})
}

type Config struct {
	Root                     string   `json:"root"`
	RequireParentApprovalFor []string `json:"require_parent_approval_for,omitempty"`
}

type Source struct {
	root            string
	requireApproval map[string]bool
	archive         *archive.Archive
	profiles        *profile.Store
}

func NewSource(cfg Config) (*Source, error) {
	if cfg.Root == "" {
		cfg.Root = "."
	}
	ar, err := archive.New(cfg.Root)
	if err != nil {
		return nil, err
	}
	ps, err := profile.NewStore(cfg.Root)
	if err != nil {
		return nil, err
	}
	req := map[string]bool{}
	for _, reason := range cfg.RequireParentApprovalFor {
		req[reason] = true
	}
	return &Source{root: cfg.Root, requireApproval: req, archive: ar, profiles: ps}, nil
}

func (s *Source) Tools(context.Context, coretools.TurnInfo) ([]coretools.Tool, error) {
	names := []string{
		"adaptive_profile_read", "adaptive_profile_update", "adaptive_archive_add",
		"adaptive_archive_top", "adaptive_outcome_record", "adaptive_experiment_create",
		"adaptive_experiment_run", "adaptive_inspect", "adaptive_report",
	}
	out := make([]coretools.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, tool{name: name, source: s})
	}
	return out, nil
}

type tool struct {
	name   string
	source *Source
}

func (t tool) Name() string { return t.name }

func (t tool) Schema() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.name,
			"description": "Adaptive experiment engine operation",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	})
	return data
}

func (t tool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	switch t.name {
	case "adaptive_profile_read":
		var in struct {
			ChildID string `json:"child_id"`
		}
		_ = json.Unmarshal(args, &in)
		p, err := t.source.profiles.Read(ctx, in.ChildID)
		return encode(p), err
	case "adaptive_profile_update":
		var in struct {
			ChildID string `json:"child_id"`
			Field   string `json:"field"`
			Value   string `json:"value"`
			Reason  string `json:"reason"`
		}
		_ = json.Unmarshal(args, &in)
		ch, err := t.source.profiles.Propose(ctx, profile.Change{ChildID: in.ChildID, Field: in.Field, Proposed: in.Value})
		if err != nil {
			return "", err
		}
		if !t.source.requireApproval[in.Reason] {
			err = t.source.profiles.Resolve(ctx, ch.ID, profile.Approved, "")
		}
		return encode(ch), err
	case "adaptive_archive_add":
		var in adaptive.AdaptiveArtifact
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		return `{"ok":true}`, t.source.archive.AddArtifact(ctx, in)
	case "adaptive_archive_top":
		var in archive.Query
		_ = json.Unmarshal(args, &in)
		top, err := t.source.archive.Top(ctx, in, 5)
		return encode(top), err
	case "adaptive_outcome_record":
		var in adaptive.AdaptiveEvalResult
		if err := json.Unmarshal(args, &in); err != nil {
			return "", err
		}
		return `{"ok":true}`, t.source.archive.AddOutcome(ctx, in)
	case "adaptive_inspect", "adaptive_report":
		var in reports.InspectQuery
		_ = json.Unmarshal(args, &in)
		path, err := reports.Inspect(ctx, t.source.root, t.source.archive, in)
		return encode(map[string]string{"path": path}), err
	case "adaptive_experiment_create", "adaptive_experiment_run":
		return encode(map[string]string{"status": "created", "note": "domain-specific execution is provided by Phase 13/14 adapters"}), nil
	default:
		return "", fmt.Errorf("unknown adaptive tool %q", t.name)
	}
}

func encode(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
