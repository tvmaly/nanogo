// Package research exposes a deterministic lesson_research tool.
package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/tools"
)

type Config struct {
	Workspace string          `json:"workspace"`
	Enabled   bool            `json:"enabled"`
	Driver    string          `json:"driver"`
	WebSearch WebSearchConfig `json:"web_search"`
	Clock     func() time.Time
}

type WebSearchConfig struct {
	Engine          string   `json:"engine"`
	MaxResults      int      `json:"max_results"`
	AllowedDomains  []string `json:"allowed_domains"`
	ExcludedDomains []string `json:"excluded_domains"`
}

type Request struct {
	Topic      string `json:"topic"`
	ChildAge   int    `json:"child_age"`
	SkillType  string `json:"skill_type"`
	MaxSources int    `json:"max_sources"`
}

type Summary struct {
	SchemaVersion           string   `json:"schema_version"`
	Topic                   string   `json:"topic"`
	SourcesPath             string   `json:"sources_path"`
	Guides                  int      `json:"guides"`
	Videos                  int      `json:"videos"`
	NeedsParentVerification []string `json:"needs_parent_verification,omitempty"`
	ProviderRequestMutation any      `json:"provider_request_mutation,omitempty"`
}

type Source struct {
	cfg Config
}

func NewSource(cfg Config) Source {
	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}
	if cfg.Driver == "" {
		cfg.Driver = "fake"
	}
	if cfg.WebSearch.MaxResults == 0 {
		cfg.WebSearch.MaxResults = 8
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	return Source{cfg: cfg}
}

func (s Source) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	if !s.cfg.Enabled {
		return nil, nil
	}
	return []tools.Tool{lessonResearchTool{source: s}}, nil
}

type lessonResearchTool struct {
	source Source
}

func (lessonResearchTool) Name() string { return "lesson_research" }

func (lessonResearchTool) Schema() json.RawMessage {
	data, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]any{"name": "lesson_research", "parameters": map[string]any{"type": "object"}}})
	return data
}

func (t lessonResearchTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var req Request
	if err := json.Unmarshal(args, &req); err != nil {
		return "", err
	}
	out, err := t.source.Research(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(out)
	return string(data), nil
}

func (s Source) Research(_ context.Context, req Request) (Summary, error) {
	if req.Topic == "" {
		return Summary{}, fmt.Errorf("topic is required")
	}
	if req.SkillType == "" {
		req.SkillType = "physical"
	}
	if req.MaxSources == 0 {
		req.MaxSources = s.cfg.WebSearch.MaxResults
	}
	path := filepath.Join(s.cfg.Workspace, "inbox", "lessons", "sources", slug(req.Topic)+".sources.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Summary{}, err
	}
	body := renderSources(req, s.cfg.Clock())
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return Summary{}, err
	}
	return Summary{
		SchemaVersion:           "research.summary.v1",
		Topic:                   req.Topic,
		SourcesPath:             path,
		Guides:                  2,
		Videos:                  2,
		NeedsParentVerification: []string{"vid-basic-throw:model_inferred"},
		ProviderRequestMutation: s.ProviderWebSearchMutation(),
	}, nil
}

func (s Source) ProviderWebSearchMutation() map[string]any {
	params := map[string]any{
		"type":        "openrouter:web_search",
		"max_results": s.cfg.WebSearch.MaxResults,
	}
	if len(s.cfg.WebSearch.AllowedDomains) > 0 {
		params["allowed_domains"] = sorted(s.cfg.WebSearch.AllowedDomains)
	}
	if len(s.cfg.WebSearch.ExcludedDomains) > 0 {
		params["excluded_domains"] = sorted(s.cfg.WebSearch.ExcludedDomains)
	}
	return params
}

func renderSources(req Request, generated time.Time) string {
	return fmt.Sprintf(`---
schema_version: research.sources.v1
topic: %s
child_age: %d
skill_type: %s
generated: %s
---
## Guides
- title: Beginner Yo-Yo Safety
  url: https://example.invalid/yoyo-safety
  summary: Clear space and use a responsive beginner yo-yo.
  trust_note: Fixture source for deterministic tests.
  selected_because: Age-appropriate safety setup.
  age_fit_note: Appropriate for a 7-year-old with parent nearby.
- title: Basic Throw Guide
  url: https://example.invalid/basic-throw
  summary: Introduces straight throws before sleeper attempts.
  trust_note: Fixture source for deterministic tests.
  selected_because: Uses beginner language.
  age_fit_note: Short and concrete.

## Videos
- video_id: vid-basic-throw
  title: Basic throw slow replay
  url: https://www.youtube.com/watch?v=vid-basic-throw
  selected_because: Clear beginner throw demonstration with slow replay.
  age_fit_note: Appropriate for a 7-year-old with parent nearby.
  skill_progression_note: Teaches the basic throw before sleeper or walk-the-dog.
  parent_check_required: true
  segments:
    - concept: the basic throw
      start: 00:42
      end: 02:10
      source: model_inferred
  trust_note: Parent should verify inferred segment.
- video_id: vid-sleeper
  title: Sleeper setup
  url: https://www.youtube.com/watch?v=vid-sleeper
  selected_because: Builds on a straight throw.
  age_fit_note: Appropriate after the throw lesson.
  skill_progression_note: Second trick in the sequence.
  parent_check_required: false
  segments:
    - concept: sleeper setup
      start: 00:15
      end: 01:00
      source: chapters
  trust_note: Fixture source for deterministic tests.

## Safety notes
- Ensure clear space, no breakables, and a responsive yo-yo for beginners.
`, req.Topic, req.ChildAge, req.SkillType, generated.Format(time.RFC3339))
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
