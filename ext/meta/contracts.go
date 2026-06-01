// Package meta owns executable lesson artifact contracts and deterministic
// fake-backed runs for the meta research loop.
package meta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	LessonBundleSchema = "meta.lesson_bundle.v1"
	RecordSchema       = "meta.record.v1"

	KindManimLesson       = "manim_lesson"
	KindBrowserGameLesson = "browser_game_lesson"

	DecisionAccepted       = "accepted"
	DecisionRejected       = "rejected"
	DecisionBudgetExceeded = "budget_exceeded"
)

var validStatuses = map[string]bool{
	"draft": true, "built": true, "validated": true, "accepted": true, "rejected": true,
}

// LessonBundle is the manifest for one generated executable lesson candidate.
type LessonBundle struct {
	SchemaVersion      string        `json:"schema_version" yaml:"schema_version"`
	ID                 string        `json:"id" yaml:"id"`
	LessonID           string        `json:"lesson_id" yaml:"lesson_id"`
	Kind               string        `json:"kind" yaml:"kind"`
	Title              string        `json:"title" yaml:"title"`
	Prompt             string        `json:"prompt" yaml:"prompt"`
	Status             string        `json:"status" yaml:"status"`
	LearningObjectives []string      `json:"learning_objectives" yaml:"learning_objectives"`
	Artifacts          []ArtifactRef `json:"artifacts" yaml:"artifacts"`
	Evidence           []EvidenceRef `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Promotion          PromotionInfo `json:"promotion" yaml:"promotion"`
}

type PromotionInfo struct {
	Eligible bool   `json:"eligible" yaml:"eligible"`
	Promoted bool   `json:"promoted" yaml:"promoted"`
	RunID    string `json:"run_id,omitempty" yaml:"run_id,omitempty"`
}

// ArtifactRef describes a generated artifact without requiring terminal parsing.
type ArtifactRef struct {
	ID       string            `json:"id" yaml:"id"`
	Kind     string            `json:"kind" yaml:"kind"`
	Path     string            `json:"path,omitempty" yaml:"path,omitempty"`
	URL      string            `json:"url,omitempty" yaml:"url,omitempty"`
	MIME     string            `json:"mime,omitempty" yaml:"mime,omitempty"`
	SHA256   string            `json:"sha256,omitempty" yaml:"sha256,omitempty"`
	Size     int64             `json:"size,omitempty" yaml:"size,omitempty"`
	RunID    string            `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	Required bool              `json:"required,omitempty" yaml:"required,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// EvidenceRef describes repair-ready diagnostic evidence.
type EvidenceRef struct {
	ID       string            `json:"id" yaml:"id"`
	Kind     string            `json:"kind" yaml:"kind"`
	Path     string            `json:"path,omitempty" yaml:"path,omitempty"`
	RunID    string            `json:"run_id" yaml:"run_id"`
	StepID   string            `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Summary  string            `json:"summary" yaml:"summary"`
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type ExtensionContract struct {
	ID                string   `json:"id" yaml:"id"`
	Profile           string   `json:"profile" yaml:"profile"`
	TemplatePath      string   `json:"template_path" yaml:"template_path"`
	Toolchain         []string `json:"toolchain" yaml:"toolchain"`
	RequiredArtifacts []string `json:"required_artifacts" yaml:"required_artifacts"`
	DefaultEvalGate   string   `json:"default_eval_gate" yaml:"default_eval_gate"`
	MutationTargets   []string `json:"mutation_targets,omitempty" yaml:"mutation_targets,omitempty"`
	Location          string   `json:"location,omitempty" yaml:"location,omitempty"`
}

type ExperimentRun struct {
	ID             string            `json:"id" yaml:"id"`
	SchemaVersion  string            `json:"schema_version" yaml:"schema_version"`
	Kind           string            `json:"kind" yaml:"kind"`
	LessonID       string            `json:"lesson_id" yaml:"lesson_id"`
	Workspace      string            `json:"workspace" yaml:"workspace"`
	RunDir         string            `json:"run_dir" yaml:"run_dir"`
	MaxWallTime    time.Duration     `json:"max_wall_time" yaml:"max_wall_time"`
	Decision       string            `json:"decision" yaml:"decision"`
	Artifacts      []ArtifactRef     `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Evidence       []EvidenceRef     `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	StartedAt      time.Time         `json:"started_at" yaml:"started_at"`
	CompletedAt    time.Time         `json:"completed_at" yaml:"completed_at"`
	Runner         string            `json:"runner" yaml:"runner"`
	FailureReasons []string          `json:"failure_reasons,omitempty" yaml:"failure_reasons,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type EvalGate struct {
	ID               string   `json:"id" yaml:"id"`
	Profile          string   `json:"profile" yaml:"profile"`
	RequiredKinds    []string `json:"required_kinds" yaml:"required_kinds"`
	MaxConsoleErrors int      `json:"max_console_errors" yaml:"max_console_errors"`
}

type LineageRecord struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Actor         string    `json:"actor"`
	Event         string    `json:"event"`
	LessonID      string    `json:"lesson_id,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	EvalGateID    string    `json:"eval_gate_id,omitempty"`
	ArtifactIDs   []string  `json:"artifact_ids,omitempty"`
	EvidenceIDs   []string  `json:"evidence_ids,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

type GraphEdge struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	Relation      string    `json:"relation"`
	RunID         string    `json:"run_id,omitempty"`
	EvidenceIDs   []string  `json:"evidence_ids,omitempty"`
}

type TryItExperiment struct {
	SchemaVersion string   `json:"schema_version" yaml:"schema_version"`
	ID            string   `json:"id" yaml:"id"`
	Kind          string   `json:"kind" yaml:"kind"`
	RunID         string   `json:"run_id" yaml:"run_id"`
	ArtifactIDs   []string `json:"artifact_ids,omitempty" yaml:"artifact_ids,omitempty"`
}

func ValidateLessonBundle(workspace string, b LessonBundle) error {
	var problems []string
	if b.SchemaVersion != LessonBundleSchema {
		problems = append(problems, "schema_version must be "+LessonBundleSchema)
	}
	if b.ID == "" {
		problems = append(problems, "id is required")
	}
	if b.LessonID == "" {
		problems = append(problems, "lesson_id is required")
	}
	if b.Kind != KindManimLesson && b.Kind != KindBrowserGameLesson {
		problems = append(problems, "kind is unsupported")
	}
	if !validStatuses[b.Status] {
		problems = append(problems, "status is unsupported")
	}
	if len(b.LearningObjectives) == 0 {
		problems = append(problems, "learning_objectives requires at least one objective")
	}
	for _, a := range b.Artifacts {
		if err := ValidateArtifactRef(workspace, a); err != nil {
			problems = append(problems, fmt.Sprintf("artifact %q: %v", a.ID, err))
		}
	}
	for _, kind := range requiredArtifactKinds(b.Kind) {
		if !hasArtifactKind(b.Artifacts, kind) {
			problems = append(problems, "missing required artifact kind "+kind)
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ValidateArtifactRef(workspace string, a ArtifactRef) error {
	if a.ID == "" {
		return errors.New("stable artifact id is required")
	}
	if a.Kind == "" {
		return errors.New("kind is required")
	}
	if a.Path == "" {
		return nil
	}
	return validateWorkspacePath(workspace, a.Path)
}

func ValidateEvidenceRef(workspace string, e EvidenceRef) error {
	if e.ID == "" {
		return errors.New("stable evidence id is required")
	}
	if e.RunID == "" {
		return errors.New("run_id is required")
	}
	if e.Path == "" {
		return nil
	}
	return validateWorkspacePath(workspace, e.Path)
}

func ValidateExtensionContract(workspace string, c ExtensionContract) error {
	var problems []string
	if c.ID == "" {
		problems = append(problems, "id is required")
	}
	if c.Profile != KindManimLesson && c.Profile != KindBrowserGameLesson {
		problems = append(problems, "profile is unsupported")
	}
	if c.TemplatePath == "" {
		problems = append(problems, "template_path is required")
	} else if err := validateWorkspacePath(workspace, c.TemplatePath); err != nil {
		problems = append(problems, "template_path path guard violation: "+err.Error())
	}
	if len(c.Toolchain) == 0 {
		problems = append(problems, "toolchain is required")
	}
	if len(c.RequiredArtifacts) == 0 {
		problems = append(problems, "required_artifacts is required")
	}
	if c.DefaultEvalGate == "" {
		problems = append(problems, "default_eval_gate is required")
	}
	if c.Location == "core" {
		problems = append(problems, "concrete artifact toolchains belong outside the core kernel")
	}
	for _, target := range c.MutationTargets {
		if target == "model_weights" {
			problems = append(problems, "model weight mutation targets are rejected")
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ValidateRunGate(workspace string, run ExperimentRun, bundle LessonBundle) (string, []string) {
	var reasons []string
	if run.Decision == DecisionBudgetExceeded {
		return DecisionBudgetExceeded, []string{"wall-time budget exceeded"}
	}
	if err := ValidateLessonBundle(workspace, bundle); err != nil {
		reasons = append(reasons, err.Error())
	}
	if bundle.Kind == KindManimLesson {
		if !existingNonEmptyArtifact(workspace, bundle.Artifacts, "video") {
			reasons = append(reasons, "missing video artifact")
		}
		if !existingArtifact(workspace, bundle.Artifacts, "log", "stream", "stdout") || !existingArtifact(workspace, bundle.Artifacts, "log", "stream", "stderr") {
			reasons = append(reasons, "missing render logs")
		}
	}
	if bundle.Kind == KindBrowserGameLesson {
		if !existingArtifact(workspace, bundle.Artifacts, "html_app", "", "") {
			reasons = append(reasons, "missing static app")
		}
		if !existingArtifact(workspace, bundle.Artifacts, "screenshot", "", "") {
			reasons = append(reasons, "missing screenshot artifact")
		}
		if countConsoleErrors(bundle.Evidence) > 0 {
			reasons = append(reasons, "browser console errors")
		}
		if hasEvidenceKind(bundle.Evidence, "happy_path_failure") {
			reasons = append(reasons, "happy-path test failed")
		}
	}
	if len(reasons) > 0 {
		return DecisionRejected, reasons
	}
	return DecisionAccepted, nil
}

func requiredArtifactKinds(kind string) []string {
	switch kind {
	case KindManimLesson:
		return []string{"video", "validation_report", "preview_page"}
	case KindBrowserGameLesson:
		return []string{"html_app", "preview_url", "screenshot", "validation_report"}
	default:
		return nil
	}
}

func hasArtifactKind(artifacts []ArtifactRef, kind string) bool {
	for _, a := range artifacts {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

func validateWorkspacePath(workspace, p string) error {
	if workspace == "" {
		workspace = "."
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	candidate := p
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	clean, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path guard violation: %s escapes workspace %s", p, workspace)
	}
	return nil
}

func existingNonEmptyArtifact(workspace string, artifacts []ArtifactRef, kind string) bool {
	for _, a := range artifacts {
		if a.Kind != kind || a.Path == "" {
			continue
		}
		info, err := os.Stat(workspacePath(workspace, a.Path))
		if err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

func existingArtifact(workspace string, artifacts []ArtifactRef, kind, metaKey, metaValue string) bool {
	for _, a := range artifacts {
		if a.Kind != kind {
			continue
		}
		if metaKey != "" && a.Metadata[metaKey] != metaValue {
			continue
		}
		if a.Path == "" && a.URL != "" {
			return true
		}
		if a.Path == "" {
			continue
		}
		if _, err := os.Stat(workspacePath(workspace, a.Path)); err == nil {
			return true
		}
	}
	return false
}

func countConsoleErrors(evidence []EvidenceRef) int {
	var n int
	for _, e := range evidence {
		if e.Kind == "console_error" {
			n++
		}
	}
	return n
}

func hasEvidenceKind(evidence []EvidenceRef, kind string) bool {
	for _, e := range evidence {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func workspacePath(workspace, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(workspace, p)
}

func artifactIDs(artifacts []ArtifactRef) []string {
	ids := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		ids = append(ids, a.ID)
	}
	sort.Strings(ids)
	return ids
}

func evidenceIDs(evidence []EvidenceRef) []string {
	ids := make([]string, 0, len(evidence))
	for _, e := range evidence {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func fileArtifact(workspace, runID, id, kind, relPath, mime string, required bool, metadata map[string]string) ArtifactRef {
	a := ArtifactRef{ID: id, Kind: kind, Path: relPath, MIME: mime, RunID: runID, Required: required, Metadata: metadata}
	data, err := os.ReadFile(workspacePath(workspace, relPath))
	if err == nil {
		sum := sha256.Sum256(data)
		a.SHA256 = hex.EncodeToString(sum[:])
		a.Size = int64(len(data))
	}
	return a
}

func urlArtifact(runID, id, kind, url string, required bool) ArtifactRef {
	return ArtifactRef{ID: id, Kind: kind, URL: url, RunID: runID, Required: required}
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = strings.Trim(re.ReplaceAllString(s, "-"), "-")
	if s == "" {
		return "lesson"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

type EvidenceStore interface {
	AppendLineage(context.Context, LineageRecord) error
	AppendGraph(context.Context, GraphEdge) error
	AppendRun(context.Context, ExperimentRun) error
	AppendEvidence(context.Context, EvidenceRef) error
}
