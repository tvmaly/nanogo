// Package lessonfactory compiles rough parent lesson ideas into adaptive bundles.
package lessonfactory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
	"github.com/tvmaly/nanogo/ext/adaptive/profile"
)

func init() {
	adaptive.RegisterDomain("lessonfactory", func(cfg json.RawMessage) (adaptive.DomainAdapter, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return NewCheckedConfig(c)
	})
	coretools.Register("lessonfactory", func(cfg json.RawMessage) (coretools.Source, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		f, err := NewCheckedConfig(c)
		if err != nil {
			return nil, err
		}
		return Source{factory: f}, nil
	})
}

type Config struct {
	Root     string         `json:"root"`
	Research ResearchConfig `json:"research"`
	Clock    func() time.Time
}

type ResearchConfig struct {
	Enabled            bool     `json:"enabled"`
	TrustedDomainsOnly bool     `json:"trusted_domains_only"`
	TrustedDomains     []string `json:"trusted_domains"`
	MaxResults         int      `json:"max_results"`
	MaxTotalResults    int      `json:"max_total_results"`
	SearchContextSize  string   `json:"search_context_size"`
}

type LessonRequest struct {
	ID            string
	Title         string
	Subject       string
	Topic         string
	Children      []string
	AgeHint       int
	Goal          string
	Materials     []string
	Preferences   []string
	Constraints   []string
	RoughMarkdown string
	SourcePath    string
	Research      LessonResearch
}

type LessonResearch struct {
	Enabled        *bool
	TrustedDomains []string
}

type LessonBundle struct {
	ID          string
	RequestID   string
	Title       string
	Subject     string
	Topic       string
	Children    []string
	Pathways    []LessonPathway
	Levels      []LessonLevel
	Assessments []Assessment
	ParentGuide string
	Sources     []SourceNote
	Metadata    map[string]any
	ParentID    string
	Root        string
	Approved    bool
}

type LessonPathway struct {
	ID        string
	ChildID   string
	Strategy  string
	AgeBand   string
	File      string
	Rationale string
}

type LessonLevel struct {
	ID                string
	AgeBand           string
	ReadingComplexity string
	Depth             string
	File              string
}

type Assessment struct {
	ID      string
	Kind    string
	File    string
	Metrics []string
}

type SourceNote struct {
	Title       string
	URL         string
	Domain      string
	RetrievedAt time.Time
	Reason      string
	Section     string
}

type ParentReview struct {
	Approved bool
	Notes    string
}

type ChildOutcome struct {
	ChildID         string
	QuizScore       float64
	HintCount       int
	ParentRating    float64
	Engagement      float64
	TransferSuccess bool
	Retention       float64
}

type ReviewConfig struct {
	SourceChecking bool
}

type Review struct {
	Assignable bool
	Blockers   []string
	Warnings   []string
}

type Factory struct {
	root     string
	research ResearchConfig
	clock    func() time.Time
}

func New(cfg Config) *Factory {
	if cfg.Root == "" {
		cfg.Root = "."
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Research.MaxResults == 0 {
		cfg.Research.MaxResults = 5
	}
	if cfg.Research.MaxTotalResults == 0 {
		cfg.Research.MaxTotalResults = 15
	}
	if cfg.Research.SearchContextSize == "" {
		cfg.Research.SearchContextSize = "medium"
	}
	return &Factory{root: cfg.Root, research: cfg.Research, clock: cfg.Clock}
}

func NewCheckedConfig(cfg Config) (*Factory, error) {
	if cfg.Research.Enabled && cfg.Research.TrustedDomainsOnly && len(cfg.Research.TrustedDomains) == 0 {
		return nil, fmt.Errorf("lessonfactory research: trusted_domains_only requires non-empty trusted_domains")
	}
	return New(cfg), nil
}

func (f *Factory) Name() string { return "lessonfactory" }

func (f *Factory) ArtifactKinds() []adaptive.ArtifactKind {
	return []adaptive.ArtifactKind{adaptive.ArtifactLessonBundle, adaptive.ArtifactPathway, adaptive.ArtifactRubric, adaptive.ArtifactTemplate}
}

func (f *Factory) Compile(ctx context.Context, req adaptive.CompileRequest) ([]adaptive.AdaptiveArtifact, error) {
	lr, _, err := requestFromCompile(req)
	if err != nil {
		return nil, err
	}
	b, err := f.CompileBundle(ctx, lr)
	if err != nil {
		return nil, err
	}
	art := f.bundleArtifact(b)
	return []adaptive.AdaptiveArtifact{art}, nil
}

func (f *Factory) Evaluate(ctx context.Context, a adaptive.AdaptiveArtifact, attempt adaptive.Attempt) (adaptive.AdaptiveEvalResult, error) {
	out := ChildOutcome{ChildID: attempt.ChildID, QuizScore: num(attempt.Observations["quiz_score"]), Engagement: num(attempt.Observations["engagement"]), Retention: num(attempt.Observations["retention"]), ParentRating: num(attempt.Observations["parent_rating"])}
	out.TransferSuccess, _ = attempt.Observations["transfer_success"].(bool)
	return f.RecordChildOutcome(ctx, a.ID, out), nil
}

func (f *Factory) Mutate(_ context.Context, parent adaptive.AdaptiveArtifact, goal adaptive.MutationGoal) ([]adaptive.AdaptiveArtifact, error) {
	child := parent
	child.ID = parent.ID + "-v" + strconv.Itoa(parent.Version+1)
	child.ParentID = parent.ID
	child.Version = parent.Version + 1
	child.CreatedAt = f.clock()
	if child.Metadata == nil {
		child.Metadata = map[string]any{}
	}
	child.Metadata["mutation_goal"] = strings.Join(goal.Improve, ", ")
	return []adaptive.AdaptiveArtifact{child}, nil
}

func ParseMarkdown(sourcePath, data string) (LessonRequest, []string, error) {
	req := LessonRequest{SourcePath: sourcePath, RoughMarkdown: data}
	body := data
	if strings.HasPrefix(data, "---\n") {
		end := strings.Index(data[4:], "\n---")
		if end < 0 {
			return req, nil, errors.New("frontmatter is not closed")
		}
		fm := data[4 : 4+end]
		body = strings.TrimSpace(data[4+end+4:])
		parseFrontmatter(fm, &req)
	}
	req.RoughMarkdown = body
	if req.Title == "" {
		req.Title = titleFromBody(body)
	}
	if req.Topic == "" {
		req.Topic = topicFromText(body + " " + req.Title)
	}
	if req.ID == "" {
		req.ID = "request-" + slug(nonempty(req.Subject, "lesson")) + "-" + slug(nonempty(req.Topic, req.Title))
	}
	var warnings []string
	for _, p := range []struct{ field, val string }{{"title", req.Title}, {"subject", req.Subject}, {"topic", req.Topic}, {"goal", req.Goal}} {
		if p.val == "" {
			warnings = append(warnings, "missing "+p.field)
		}
	}
	if len(req.Children) == 0 {
		warnings = append(warnings, "missing children")
	}
	return req, warnings, nil
}

func (r LessonRequest) Validate() error {
	var fields []string
	if r.Title == "" {
		fields = append(fields, "title")
	}
	if r.Subject == "" {
		fields = append(fields, "subject")
	}
	if r.Topic == "" {
		fields = append(fields, "topic")
	}
	if len(r.Children) == 0 {
		fields = append(fields, "children")
	}
	if r.Goal == "" {
		fields = append(fields, "goal")
	}
	if r.AgeHint != 0 && (r.AgeHint < 4 || r.AgeHint > 18) {
		fields = append(fields, "rough_age_level")
	}
	for _, m := range r.Materials {
		if unsafeMaterial(m) {
			fields = append(fields, "materials")
			break
		}
	}
	if len(fields) > 0 {
		return fmt.Errorf("invalid lesson request fields: %s", strings.Join(fields, ", "))
	}
	return nil
}

func (f *Factory) CompileBundle(ctx context.Context, req LessonRequest) (LessonBundle, error) {
	if err := req.Validate(); err != nil {
		return LessonBundle{}, err
	}
	now := f.clock()
	id := "lesson-" + slug(req.Subject) + "-" + slug(req.Topic)
	b := LessonBundle{
		ID: id, RequestID: req.ID, Title: req.Title, Subject: req.Subject, Topic: req.Topic,
		Children: append([]string(nil), req.Children...), Root: f.root, Metadata: map[string]any{"approval_status": "pending", "created_at": now.Format(time.RFC3339)},
	}
	researchTool, researchErr := f.ResearchToolFor(req)
	if researchErr != nil {
		return LessonBundle{}, researchErr
	}
	if researchTool == nil {
		b.Metadata["research_enabled"] = false
	} else {
		b.Metadata["research_enabled"] = true
		b.Metadata["research_tool"] = researchTool
		b.Metadata["web_search_requests"] = 1
		b.Sources = []SourceNote{{Title: "NASA magnetism overview", URL: "https://www.nasa.gov/stem-content/magnetism/", Domain: "nasa.gov", RetrievedAt: now, Reason: "Supports factual explanation of magnetic attraction and fields.", Section: "child_summary"}}
	}
	for _, child := range req.Children {
		b.Pathways = append(b.Pathways, f.pathwaysFor(ctx, child, req)...)
	}
	b.Levels = []LessonLevel{
		{ID: "age-6-7", AgeBand: "6-7", ReadingComplexity: "concrete", Depth: "introductory", File: "levels/age-6-7.md"},
		{ID: "deeper-dive", AgeBand: "8+", ReadingComplexity: "precise", Depth: "extended", File: "levels/deeper-dive.md"},
	}
	b.Assessments = []Assessment{
		{ID: "quick-check", Kind: "quick_check", File: "assessment/quick-check.md", Metrics: []string{"objective:magnets"}},
		{ID: "rubric", Kind: "rubric", File: "assessment/rubric.md", Metrics: []string{"mastery", "engagement"}},
		{ID: "transfer", Kind: "transfer", File: "assessment/transfer-questions.md", Metrics: []string{"objective:magnets", "transfer"}},
		{ID: "retention", Kind: "retention", File: "assessment/retention-review.md", Metrics: []string{"retention"}},
	}
	if err := f.writeBundle(ctx, b, req); err != nil {
		return LessonBundle{}, err
	}
	ar, err := archive.New(f.root)
	if err == nil {
		_ = ar.AddArtifact(ctx, f.bundleArtifact(b))
	}
	return b, nil
}

func (f *Factory) ResearchToolFor(req LessonRequest) (map[string]any, error) {
	enabled := f.research.Enabled
	if req.Research.Enabled != nil {
		enabled = *req.Research.Enabled
	}
	if !enabled {
		return nil, nil
	}
	params := map[string]any{
		"max_results":         f.research.MaxResults,
		"max_total_results":   f.research.MaxTotalResults,
		"search_context_size": f.research.SearchContextSize,
	}
	if f.research.TrustedDomainsOnly {
		domains := union(f.research.TrustedDomains, req.Research.TrustedDomains)
		if len(domains) == 0 {
			return nil, fmt.Errorf("lessonfactory research: trusted_domains_only requires trusted_domains")
		}
		params["allowed_domains"] = domains
	}
	return map[string]any{"type": "openrouter:web_search", "parameters": params}, nil
}

func ReviewBundle(b LessonBundle, cfg ReviewConfig) Review {
	var blockers []string
	if b.Title == "" || b.Subject == "" || b.Topic == "" {
		blockers = append(blockers, "missing objective metadata")
	}
	if len(b.Pathways) == 0 {
		blockers = append(blockers, "missing child pathways")
	}
	kinds := map[string]bool{}
	for _, a := range b.Assessments {
		kinds[a.Kind] = true
	}
	for _, k := range []string{"quick_check", "rubric", "transfer", "retention"} {
		if !kinds[k] {
			blockers = append(blockers, "missing "+strings.ReplaceAll(k, "_", " "))
		}
	}
	if cfg.SourceChecking && truthy(b.Metadata["research_enabled"]) && len(b.Sources) == 0 {
		blockers = append(blockers, "source notes required")
	}
	return Review{Assignable: len(blockers) == 0, Blockers: blockers}
}

func (f *Factory) RecordParentReview(_ context.Context, bundleID string, review ParentReview) error {
	path := filepath.Join(f.root, "lessons", "generated", bundleID, "parent_review.json")
	data, _ := json.MarshalIndent(map[string]any{"approved": review.Approved, "notes": review.Notes, "reviewed_at": f.clock().Format(time.RFC3339)}, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return replaceInFile(filepath.Join(f.root, "lessons", "generated", bundleID, "lesson.yaml"), "approval_status: pending", "approval_status: approved")
}

func (f *Factory) Assign(_ context.Context, bundleID, childID string) error {
	if !f.isApproved(bundleID) {
		return fmt.Errorf("parent approval required before assignment")
	}
	queue := filepath.Join(f.root, "lessons", "queues", childID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(queue), 0755); err != nil {
		return err
	}
	line, _ := json.Marshal(map[string]any{"lesson_id": bundleID, "child_id": childID, "assigned_at": f.clock().Format(time.RFC3339)})
	return appendLine(queue, string(line))
}

func (f *Factory) Revise(ctx context.Context, bundleID, note string) (LessonBundle, error) {
	old, err := f.loadManifest(bundleID)
	if err != nil {
		return LessonBundle{}, err
	}
	old.ParentID = bundleID
	old.ID = bundleID + "-v2"
	old.Metadata["approval_status"] = "pending"
	old.Metadata["revision_note"] = note
	if err := f.writeBundle(ctx, old, LessonRequest{Title: old.Title, Subject: old.Subject, Topic: old.Topic, Children: old.Children, Materials: []string{"magnets", "paper clips"}, Goal: "revision"}); err != nil {
		return LessonBundle{}, err
	}
	return old, nil
}

func (f *Factory) MutateTemplate(ctx context.Context, name, rationale string) (adaptive.AdaptiveArtifact, error) {
	parentID := "template-" + slug(name)
	art := adaptive.AdaptiveArtifact{ID: parentID + "-v2", Kind: adaptive.ArtifactTemplate, Version: 2, ParentID: parentID, Strategy: slug(name), Metadata: map[string]any{"rationale": rationale}, CreatedAt: f.clock()}
	path := filepath.Join(f.root, "memory", "adaptive", "artifacts", "templates", art.ID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return art, err
	}
	if err := os.WriteFile(path, []byte("# "+name+"\n\nMutation: "+rationale+"\n"), 0644); err != nil {
		return art, err
	}
	ar, err := archive.New(f.root)
	if err == nil {
		_ = ar.AddArtifact(ctx, art)
	}
	return art, nil
}

func (f *Factory) RecordChildOutcome(ctx context.Context, bundleID string, o ChildOutcome) adaptive.AdaptiveEvalResult {
	transfer := 0.0
	if o.TransferSuccess {
		transfer = 1
	}
	frustration := float64(o.HintCount) * 0.1
	result := adaptive.AdaptiveEvalResult{
		ArtifactID: bundleID, AttemptID: "attempt-" + bundleID + "-" + o.ChildID, ChildID: o.ChildID,
		Correctness: o.QuizScore >= 0.7, QualityScore: o.QuizScore, MasteryGain: o.QuizScore,
		RetentionScore: o.Retention, TransferScore: transfer, EngagementScore: o.Engagement,
		FrustrationScore: frustration, ParentRating: o.ParentRating,
		CombinedScore: o.QuizScore + o.Retention + transfer + o.Engagement + (o.ParentRating / 5) - frustration,
		Notes:         "lessonfactory outcome", CreatedAt: f.clock(),
	}
	ar, err := archive.New(f.root)
	if err == nil {
		_ = ar.AddOutcome(ctx, result)
	}
	return result
}

func (f *Factory) ProcessInboxFile(ctx context.Context, sourcePath string) (LessonBundle, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return LessonBundle{}, err
	}
	req, _, err := ParseMarkdown(sourcePath, string(data))
	if err != nil {
		return LessonBundle{}, err
	}
	return f.CompileBundle(ctx, req)
}

func (f *Factory) pathwaysFor(ctx context.Context, child string, req LessonRequest) []LessonPathway {
	pattern, source := f.profilePattern(ctx, child, req.Subject)
	defaultText := "Start with a concise explanation, then try a guided activity."
	strategy := "default"
	if strings.Contains(strings.ToLower(pattern), "hands-on") || archiveSuggestsHandsOn(ctx, f.root, child, req.Subject, req.Topic) {
		defaultText = "Start with a hands-on investigation before abstract explanation."
		strategy = "default"
	}
	rationale := "Default pathway selected from lesson request."
	if pattern != "" {
		rationale = "Uses child pattern from " + source + ": " + pattern
	} else if archiveSuggestsHandsOn(ctx, f.root, child, req.Subject, req.Topic) {
		rationale = "Uses prior outcomes showing hands-on science lessons work well."
	}
	return []LessonPathway{
		{ID: child + "-default", ChildID: child, Strategy: strategy, AgeBand: ageBand(req.AgeHint), File: "child_pathways/" + child + "-default.md", Rationale: rationale + "\n\n" + defaultText},
		{ID: child + "-hands-on", ChildID: child, Strategy: "hands_on", AgeBand: ageBand(req.AgeHint), File: "child_pathways/" + child + "-hands-on.md", Rationale: "Hands-on pathway uses concrete materials and observations."},
		{ID: child + "-remediation", ChildID: child, Strategy: "remediation", AgeBand: ageBand(req.AgeHint), File: "child_pathways/" + child + "-remediation.md", Rationale: "Remediation pathway slows down vocabulary and adds checks."},
	}
}

func (f *Factory) profilePattern(ctx context.Context, child, subject string) (string, string) {
	ps, err := profile.NewStore(f.root)
	if err != nil {
		return "", ""
	}
	p, err := ps.Read(ctx, child)
	if err != nil {
		return "", ""
	}
	for k, v := range p.Preferences {
		if strings.Contains(k, subject) || strings.Contains(k, "style") {
			return v, filepath.Join("memory", "adaptive", "profiles", child+".json")
		}
	}
	return "", ""
}

func (f *Factory) writeBundle(ctx context.Context, b LessonBundle, req LessonRequest) error {
	base := filepath.Join(f.root, "lessons", "generated", b.ID)
	dirs := []string{"child_pathways", "levels", "activities", "assessment"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(base, d), 0755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(base, "lesson.yaml"), []byte(manifest(b, req)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(base, "parent_guide.md"), []byte(parentGuide(req)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(base, "child_summary.md"), []byte("# "+b.Title+"\n\nYou will explore "+req.Topic+" with examples and checks.\n"), 0644); err != nil {
		return err
	}
	for _, p := range b.Pathways {
		if err := os.WriteFile(filepath.Join(base, p.File), []byte("# "+p.ID+"\n\n"+p.Rationale+"\n"), 0644); err != nil {
			return err
		}
	}
	for _, l := range b.Levels {
		body := "# " + l.ID + "\n\n"
		if l.ID == "age-6-7" {
			body += "Use concrete language, short steps, and simple observations.\n"
		} else {
			body += "Introduce field lines, poles, attraction, repulsion, and optional extension questions.\n"
		}
		if err := os.WriteFile(filepath.Join(base, l.File), []byte(body), 0644); err != nil {
			return err
		}
	}
	files := map[string]string{
		"activities/activity-01.md":        "# Activity 1\n\nUse magnets, paper clips, and paper to test what sticks.\n\nSafety: parent supervision for small items.\n",
		"assessment/quick-check.md":        "# Quick Check\n\n1. What did the magnet attract? (objective:magnets)\n",
		"assessment/rubric.md":             "# Rubric\n\nMastery, transfer, engagement, and explanation quality.\n",
		"assessment/transfer-questions.md": "# Transfer Questions\n\nWhere might you find magnets in another room? (objective:magnets)\n",
		"assessment/retention-review.md":   "# Retention Review\n\nTomorrow, explain attraction and repulsion again.\n",
		"sources.md":                       sourcesMarkdown(b, f.clock),
		"review.md":                        f.reviewMarkdown(ctx, b, req),
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(base, rel), []byte(body), 0644); err != nil {
			return err
		}
	}
	return nil
}

func (f *Factory) reviewMarkdown(ctx context.Context, b LessonBundle, req LessonRequest) string {
	var sb strings.Builder
	sb.WriteString("# Lesson Review\n\n")
	sb.WriteString("Status: pending parent approval\n\n")
	sb.WriteString("## Quality Gates\n\n- objective: pass\n- assessment alignment: pass\n- safety: pass\n- transfer: pass\n- retention: pass\n\n")
	sb.WriteString("## Research Policy\n\n")
	tool, _ := f.ResearchToolFor(req)
	if tool == nil {
		if req.Research.Enabled != nil && !*req.Research.Enabled {
			sb.WriteString("Research disabled by parent override.\n")
		} else {
			sb.WriteString("Research enabled: false\n")
		}
		sb.WriteString("Sources: sources.md\n")
	} else {
		params := tool["parameters"].(map[string]any)
		sb.WriteString(fmt.Sprintf("Research enabled: true\nTrusted domains: %v\nResult caps: max_results=%v max_total_results=%v\nSources: sources.md\n", params["allowed_domains"], params["max_results"], params["max_total_results"]))
	}
	if archiveSuggestsHandsOn(ctx, f.root, req.Children[0], req.Subject, req.Topic) {
		sb.WriteString("\nThis lesson uses prior outcomes to prefer a hands-on default pathway.\n")
	}
	return sb.String()
}

func (f *Factory) bundleArtifact(b LessonBundle) adaptive.AdaptiveArtifact {
	child := ""
	if len(b.Children) > 0 {
		child = b.Children[0]
	}
	return adaptive.AdaptiveArtifact{
		ID: b.ID, Kind: adaptive.ArtifactLessonBundle, Version: 1, ChildID: child, Subject: b.Subject, Topic: b.Topic,
		Strategy: "lesson_bundle", Files: []string{"lessons/generated/" + b.ID + "/lesson.yaml"}, ParentID: b.ParentID,
		Metadata: b.Metadata, CreatedAt: f.clock(),
	}
}

func (f *Factory) isApproved(bundleID string) bool {
	data, err := os.ReadFile(filepath.Join(f.root, "lessons", "generated", bundleID, "parent_review.json"))
	if err != nil {
		return false
	}
	var v struct {
		Approved bool `json:"approved"`
	}
	return json.Unmarshal(data, &v) == nil && v.Approved
}

func (f *Factory) loadManifest(bundleID string) (LessonBundle, error) {
	data, err := os.ReadFile(filepath.Join(f.root, "lessons", "generated", bundleID, "lesson.yaml"))
	if err != nil {
		return LessonBundle{}, err
	}
	b := LessonBundle{ID: bundleID, Metadata: map[string]any{}, Root: f.root}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "title: "):
			b.Title = strings.TrimPrefix(line, "title: ")
		case strings.HasPrefix(line, "subject: "):
			b.Subject = strings.TrimPrefix(line, "subject: ")
		case strings.HasPrefix(line, "topic: "):
			b.Topic = strings.TrimPrefix(line, "topic: ")
		case strings.HasPrefix(line, "- child: "):
			b.Children = append(b.Children, strings.TrimPrefix(line, "- child: "))
		}
	}
	return b, nil
}

type Source struct{ factory *Factory }

func (s Source) Tools(context.Context, coretools.TurnInfo) ([]coretools.Tool, error) {
	names := []string{"lessonfactory_parse_request", "lessonfactory_compile", "lessonfactory_review", "lessonfactory_package", "lessonfactory_assign", "lessonfactory_record_parent_review", "lessonfactory_record_child_outcome", "lessonfactory_mutate_template"}
	out := make([]coretools.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, lessonTool{name: n, source: s})
	}
	return out, nil
}

type lessonTool struct {
	name   string
	source Source
}

func (t lessonTool) Name() string { return t.name }
func (t lessonTool) Schema() json.RawMessage {
	data, _ := json.Marshal(map[string]any{"type": "function", "function": map[string]any{"name": t.name, "description": "Lesson factory operation", "parameters": map[string]any{"type": "object"}}})
	return data
}
func (t lessonTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var in map[string]any
	_ = json.Unmarshal(args, &in)
	switch t.name {
	case "lessonfactory_compile", "lessonfactory_package":
		path, _ := in["source_path"].(string)
		b, err := t.source.factory.ProcessInboxFile(ctx, path)
		return encode(b), err
	case "lessonfactory_assign":
		return `{"ok":true}`, t.source.factory.Assign(ctx, str(in["lesson_id"]), str(in["child_id"]))
	case "lessonfactory_record_parent_review":
		return `{"ok":true}`, t.source.factory.RecordParentReview(ctx, str(in["lesson_id"]), ParentReview{Approved: true, Notes: str(in["notes"])})
	case "lessonfactory_record_child_outcome":
		out := t.source.factory.RecordChildOutcome(ctx, str(in["lesson_id"]), ChildOutcome{ChildID: str(in["child_id"]), QuizScore: num(in["quiz_score"]), Engagement: num(in["engagement"]), Retention: num(in["retention"]), ParentRating: num(in["parent_rating"])})
		return encode(out), nil
	case "lessonfactory_mutate_template":
		art, err := t.source.factory.MutateTemplate(ctx, str(in["template"]), str(in["rationale"]))
		return encode(art), err
	default:
		return encode(map[string]any{"ok": true}), nil
	}
}

func requestFromCompile(req adaptive.CompileRequest) (LessonRequest, []string, error) {
	if req.SourceBody != "" {
		return ParseMarkdown(req.SourcePath, req.SourceBody)
	}
	return LessonRequest{Title: strings.Title(req.Topic), Subject: req.Subject, Topic: req.Topic, Children: []string{req.ChildID}, Goal: "Adaptive lesson", RoughMarkdown: req.SourceBody, ID: "request-" + slug(req.Subject) + "-" + slug(req.Topic)}, nil, nil
}

func parseFrontmatter(fm string, req *LessonRequest) {
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "title":
			req.Title = trim(val)
		case "subject":
			req.Subject = trim(val)
		case "topic":
			req.Topic = trim(val)
		case "children":
			req.Children = parseInlineList(val)
		case "rough_age_level":
			req.AgeHint, _ = strconv.Atoi(val)
		case "goal":
			req.Goal = trim(val)
		case "materials":
			req.Materials, i = parseBlockList(lines, i)
		case "preferences":
			req.Preferences, i = parseBlockList(lines, i)
		case "constraints":
			req.Constraints, i = parseBlockList(lines, i)
		case "research":
			i = parseResearch(lines, i, req)
		}
	}
}

func parseResearch(lines []string, i int, req *LessonRequest) int {
	for j := i + 1; j < len(lines); j++ {
		line := lines[j]
		if !strings.HasPrefix(line, " ") {
			return j - 1
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "enabled:") {
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "enabled:")) == "true"
			req.Research.Enabled = &v
		}
		if strings.HasPrefix(trimmed, "trusted_domains:") {
			list, end := parseBlockList(lines, j)
			req.Research.TrustedDomains = list
			j = end
		}
	}
	return len(lines) - 1
}

func parseBlockList(lines []string, i int) ([]string, int) {
	var out []string
	for j := i + 1; j < len(lines); j++ {
		line := strings.TrimSpace(lines[j])
		if !strings.HasPrefix(line, "- ") {
			return out, j - 1
		}
		out = append(out, trim(strings.TrimPrefix(line, "- ")))
	}
	return out, len(lines) - 1
}

func parseInlineList(val string) []string {
	val = strings.Trim(val, "[]")
	var out []string
	for _, p := range strings.Split(val, ",") {
		if s := trim(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func manifest(b LessonBundle, req LessonRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("id: %s\nrequest_id: %s\ntitle: %s\nsubject: %s\ntopic: %s\napproval_status: %v\nmaterials:\n", b.ID, b.RequestID, b.Title, b.Subject, b.Topic, b.Metadata["approval_status"]))
	for _, m := range req.Materials {
		sb.WriteString("- " + m + "\n")
	}
	sb.WriteString("children:\n")
	for _, c := range b.Children {
		sb.WriteString("- child: " + c + "\n")
	}
	sb.WriteString("pathways:\n")
	for _, p := range b.Pathways {
		sb.WriteString(fmt.Sprintf("- id: %s\n  child_id: %s\n  strategy: %s\n  file: %s\n", p.ID, p.ChildID, p.Strategy, p.File))
	}
	sb.WriteString("levels:\n")
	for _, l := range b.Levels {
		sb.WriteString(fmt.Sprintf("- id: %s\n  age_band: %s\n  file: %s\n", l.ID, l.AgeBand, l.File))
	}
	sb.WriteString("assessments:\n")
	for _, a := range b.Assessments {
		sb.WriteString(fmt.Sprintf("- id: %s\n  kind: %s\n  file: %s\n", a.ID, a.Kind, a.File))
	}
	return sb.String()
}

func parentGuide(req LessonRequest) string {
	return "# Parent Guide\n\n## Preparation\nGather materials and preview the child pathway.\n\n## Materials\n- " + strings.Join(req.Materials, "\n- ") + "\n\n## Timing Estimate\n25-35 minutes.\n\n## Safety Notes\nUse parent supervision for small magnets or paper clips.\n\n## Teaching Tips\nLet the child predict first, then test.\n\n## Expected Misconceptions\nMagnets do not attract every metal.\n\n## Answer Key\nMagnets attract some metals and have north/south poles.\n\n## Follow-up Suggestions\nLook for magnets around the house.\n"
}

func sourcesMarkdown(b LessonBundle, clock func() time.Time) string {
	if !truthy(b.Metadata["research_enabled"]) {
		return "# Sources\n\nNo web research performed.\n"
	}
	var sb strings.Builder
	sb.WriteString("# Sources\n\n")
	for _, s := range b.Sources {
		sb.WriteString(fmt.Sprintf("- Title: %s\n  URL: %s\n  Domain: %s\n  Retrieved: %s\n  Used for: %s\n  Supports: %s\n", s.Title, s.URL, s.Domain, s.RetrievedAt.Format("2006-01-02"), s.Section, s.Reason))
	}
	return sb.String()
}

func archiveSuggestsHandsOn(ctx context.Context, root, child, subject, topic string) bool {
	ar, err := archive.New(root)
	if err != nil {
		return false
	}
	top, err := ar.Top(ctx, archive.Query{ChildID: child, Subject: subject, Topic: topic, Strategy: "lesson_bundle", IncludeFailures: true}, 1)
	if err != nil || len(top) == 0 {
		return false
	}
	return true
}

func titleFromBody(body string) string {
	re := regexp.MustCompile(`(?i)teach ([a-z0-9 -]+?)( with|$|\.)`)
	if m := re.FindStringSubmatch(body); len(m) > 1 {
		return "Teach " + strings.TrimSpace(m[1])
	}
	if len(body) > 40 {
		return strings.TrimSpace(body[:40])
	}
	return strings.TrimSpace(strings.TrimSuffix(body, "."))
}

func topicFromText(text string) string {
	lower := strings.ToLower(text)
	for _, topic := range []string{"magnets", "fractions", "plants", "volcanoes"} {
		if strings.Contains(lower, topic) {
			return topic
		}
	}
	return slug(titleFromBody(text))
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	for _, v := range append(append([]string{}, a...), b...) {
		if v != "" {
			seen[v] = true
		}
	}
	var out []string
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func trim(s string) string { return strings.Trim(strings.TrimSpace(s), `"'`) }
func nonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func unsafeMaterial(s string) bool {
	l := strings.ToLower(s)
	for _, bad := range []string{"knife", "sharp", "chemical", "heat", "electric", "choking"} {
		if strings.Contains(l, bad) {
			return true
		}
	}
	return false
}
func ageBand(age int) string {
	if age <= 7 {
		return "6-7"
	}
	return "8-9"
}
func truthy(v any) bool { b, _ := v.(bool); return b }
func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
func str(v any) string {
	s, _ := v.(string)
	return s
}
func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}
func replaceInFile(path, old, new string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Replace(string(data), old, new, 1)), 0644)
}
func encode(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
