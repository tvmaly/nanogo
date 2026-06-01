package meta

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CreateLessonRequest struct {
	Kind   string
	Prompt string
	Runner string
}

type CreateLessonResult struct {
	LessonID       string
	RunID          string
	Decision       string
	Eligible       bool
	RunDir         string
	BundlePath     string
	PreviewPath    string
	PreviewURL     string
	VideoPath      string
	ValidationPath string
	ArtifactPaths  []string
	FailureReasons []string
}

type Service struct {
	Workspace string
	Store     EvidenceStore
	Clock     func() time.Time
	Fake      FakeRunnerOptions
}

type FakeRunnerOptions struct {
	MissingVideo       bool
	MissingRenderLogs  bool
	BrowserConsoleErrs int
	HappyPathFails     bool
	BudgetExceeded     bool
}

func NewService(workspace string, store EvidenceStore) *Service {
	if store == nil {
		store = NewJSONLStore(workspace)
	}
	return &Service{Workspace: workspace, Store: store, Clock: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) CreateLesson(ctx context.Context, req CreateLessonRequest) (CreateLessonResult, error) {
	if req.Runner != "" && req.Runner != "fake" {
		return CreateLessonResult{}, fmt.Errorf("unsupported runner %q", req.Runner)
	}
	if req.Kind != KindManimLesson && req.Kind != KindBrowserGameLesson {
		return CreateLessonResult{}, fmt.Errorf("invalid lesson kind %q", req.Kind)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return CreateLessonResult{}, fmt.Errorf("--prompt is required")
	}
	if s.Workspace == "" {
		s.Workspace = "."
	}
	if err := validateWorkspacePath(s.Workspace, "lessons/generated"); err != nil {
		return CreateLessonResult{}, err
	}
	now := s.now()
	lessonID := "lesson-" + slug(req.Prompt)
	runID := "run-" + now.UTC().Format("20060102T150405Z")
	runRel := filepath.Join("lessons", "generated", lessonID, "runs", runID)
	runDir := filepath.Join(s.Workspace, runRel)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return CreateLessonResult{}, err
	}

	run := ExperimentRun{
		ID:            runID,
		SchemaVersion: RecordSchema,
		Kind:          req.Kind,
		LessonID:      lessonID,
		Workspace:     s.Workspace,
		RunDir:        runRel,
		MaxWallTime:   30 * time.Second,
		StartedAt:     now,
		Runner:        "fake",
		Metadata:      map[string]string{"prompt": req.Prompt},
	}
	if err := s.Store.AppendLineage(ctx, lineage(now, "created", lessonID, runID, "", nil, nil, "")); err != nil {
		return CreateLessonResult{}, err
	}

	bundle, result, err := s.writeFakeArtifacts(req, lessonID, runID, runRel)
	if err != nil {
		return CreateLessonResult{}, err
	}
	decision, reasons := ValidateRunGate(s.Workspace, run, bundle)
	if s.Fake.BudgetExceeded {
		decision = DecisionBudgetExceeded
		reasons = []string{"wall-time budget exceeded"}
	}
	bundle.Status = "validated"
	bundle.Promotion = PromotionInfo{Eligible: decision == DecisionAccepted, Promoted: false, RunID: runID}
	if decision != DecisionAccepted {
		bundle.Status = "rejected"
	}
	if err := writeYAML(filepath.Join(runDir, "lesson.bundle.yaml"), bundle); err != nil {
		return CreateLessonResult{}, err
	}
	run.Decision = decision
	run.Artifacts = bundle.Artifacts
	run.Evidence = bundle.Evidence
	run.CompletedAt = s.now()
	run.FailureReasons = reasons
	if err := s.Store.AppendRun(ctx, run); err != nil {
		return CreateLessonResult{}, err
	}
	if err := s.Store.AppendLineage(ctx, lineage(run.CompletedAt, "tested", lessonID, runID, "", artifactIDs(bundle.Artifacts), evidenceIDs(bundle.Evidence), "")); err != nil {
		return CreateLessonResult{}, err
	}
	event := "accepted"
	reason := ""
	if decision != DecisionAccepted {
		event = "rejected"
		reason = strings.Join(reasons, "; ")
	}
	if err := s.Store.AppendLineage(ctx, lineage(run.CompletedAt, event, lessonID, runID, evalGateID(req.Kind), artifactIDs(bundle.Artifacts), evidenceIDs(bundle.Evidence), reason)); err != nil {
		return CreateLessonResult{}, err
	}
	for _, ev := range bundle.Evidence {
		if err := s.Store.AppendEvidence(ctx, ev); err != nil {
			return CreateLessonResult{}, err
		}
	}
	if err := s.writeGraph(ctx, req.Kind, lessonID, runID, bundle, run.CompletedAt, decision); err != nil {
		return CreateLessonResult{}, err
	}
	result.Decision = decision
	result.Eligible = decision == DecisionAccepted
	result.FailureReasons = reasons
	return result, nil
}

func (s *Service) writeFakeArtifacts(req CreateLessonRequest, lessonID, runID, runRel string) (LessonBundle, CreateLessonResult, error) {
	runDir := filepath.Join(s.Workspace, runRel)
	bundle := LessonBundle{
		SchemaVersion:      LessonBundleSchema,
		ID:                 "bundle-" + lessonID + "-" + runID,
		LessonID:           lessonID,
		Kind:               req.Kind,
		Title:              titleFor(req.Kind, req.Prompt),
		Prompt:             req.Prompt,
		Status:             "draft",
		LearningObjectives: []string{"Explain the core idea", "Practice with an age-appropriate activity"},
		Promotion:          PromotionInfo{Eligible: false, Promoted: false, RunID: runID},
	}
	result := CreateLessonResult{LessonID: lessonID, RunID: runID, RunDir: runDir, BundlePath: filepath.Join(runDir, "lesson.bundle.yaml")}
	if req.Kind == KindManimLesson {
		return s.writeFakeManim(bundle, result, runRel)
	}
	return s.writeFakeBrowserGame(bundle, result, runRel)
}

func (s *Service) writeFakeManim(bundle LessonBundle, result CreateLessonResult, runRel string) (LessonBundle, CreateLessonResult, error) {
	runDir := filepath.Join(s.Workspace, runRel)
	files := map[string]string{
		"lesson.py":              "from manimlib import *\n\nclass Lesson(Scene):\n    def construct(self):\n        pass\n",
		"render.stdout.log":      "fake manim render complete\n",
		"render.stderr.log":      "",
		"preview/index.html":     "<!doctype html><title>Manim preview</title><video src='../media/lesson.mp4' controls></video>\n",
		"validation_report.json": "{\n  \"passed\": true,\n  \"checks\": [\"bundle\", \"video\", \"logs\", \"preview\"]\n}\n",
	}
	if !s.Fake.MissingVideo {
		files["media/lesson.mp4"] = "fake mp4 bytes\n"
	}
	if s.Fake.MissingRenderLogs {
		delete(files, "render.stdout.log")
	}
	if err := writeFiles(runDir, files); err != nil {
		return LessonBundle{}, CreateLessonResult{}, err
	}
	arts := []ArtifactRef{
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-source", "source", filepath.Join(runRel, "lesson.py"), "text/x-python", false, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-stdout", "log", filepath.Join(runRel, "render.stdout.log"), "text/plain", false, map[string]string{"stream": "stdout"}),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-stderr", "log", filepath.Join(runRel, "render.stderr.log"), "text/plain", false, map[string]string{"stream": "stderr"}),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-validation", "validation_report", filepath.Join(runRel, "validation_report.json"), "application/json", true, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-preview", "preview_page", filepath.Join(runRel, "preview/index.html"), "text/html", true, nil),
	}
	if !s.Fake.MissingVideo {
		arts = append(arts, fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-video", "video", filepath.Join(runRel, "media/lesson.mp4"), "video/mp4", true, nil))
		result.VideoPath = filepath.Join(runDir, "media/lesson.mp4")
	}
	bundle.Artifacts = arts
	result.PreviewPath = filepath.Join(runDir, "preview/index.html")
	result.ValidationPath = filepath.Join(runDir, "validation_report.json")
	result.ArtifactPaths = collectPaths(arts, s.Workspace)
	return bundle, result, nil
}

func (s *Service) writeFakeBrowserGame(bundle LessonBundle, result CreateLessonResult, runRel string) (LessonBundle, CreateLessonResult, error) {
	runDir := filepath.Join(s.Workspace, runRel)
	files := map[string]string{
		"package.json":            "{\n  \"scripts\": {\"build\": \"vite --host 127.0.0.1\"},\n  \"dependencies\": {}\n}\n",
		"index.html":              "<!doctype html><div id=\"app\"></div><script type=\"module\" src=\"/src/main.js\"></script>\n",
		"src/main.js":             "document.querySelector('#app').textContent = 'Sort solids, liquids, and gases';\n",
		"dist/index.html":         "<!doctype html><div>Fake static browser game build</div>\n",
		"preview/index.html":      "<!doctype html><iframe src='../dist/index.html'></iframe>\n",
		"screenshots/preview.png": "fake png bytes\n",
		"build.stdout.log":        "fake vite build complete\n",
		"build.stderr.log":        "",
		"validation_report.json":  "{\n  \"passed\": true,\n  \"console_error_count\": 0,\n  \"happy_path\": true\n}\n",
	}
	var evidence []EvidenceRef
	if s.Fake.BrowserConsoleErrs > 0 {
		files["validation_report.json"] = fmt.Sprintf("{\n  \"passed\": false,\n  \"console_error_count\": %d,\n  \"happy_path\": true\n}\n", s.Fake.BrowserConsoleErrs)
		files["console_errors.log"] = "TypeError: fake browser console error\n"
		evidence = append(evidence, EvidenceRef{ID: "evidence-" + bundle.LessonID + "-console-error", Kind: "console_error", Path: filepath.Join(runRel, "console_errors.log"), RunID: bundle.Promotion.RunID, StepID: "browser-smoke", Summary: "browser console errors captured"})
	}
	if s.Fake.HappyPathFails {
		files["validation_report.json"] = "{\n  \"passed\": false,\n  \"console_error_count\": 0,\n  \"happy_path\": false\n}\n"
		files["happy_path_failure.log"] = "expected sort action did not complete\n"
		evidence = append(evidence, EvidenceRef{ID: "evidence-" + bundle.LessonID + "-happy-path", Kind: "happy_path_failure", Path: filepath.Join(runRel, "happy_path_failure.log"), RunID: bundle.Promotion.RunID, StepID: "browser-smoke", Summary: "happy-path validation failed"})
	}
	if s.Fake.BudgetExceeded {
		files["build.stderr.log"] = "fake build stopped by wall-time budget\n"
		evidence = append(evidence, EvidenceRef{ID: "evidence-" + bundle.LessonID + "-budget", Kind: "diagnostic", Path: filepath.Join(runRel, "build.stderr.log"), RunID: bundle.Promotion.RunID, StepID: "build", Summary: "wall-time budget exceeded"})
	}
	if err := writeFiles(runDir, files); err != nil {
		return LessonBundle{}, CreateLessonResult{}, err
	}
	arts := []ArtifactRef{
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-source", "source", filepath.Join(runRel, "src/main.js"), "text/javascript", false, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-app", "html_app", filepath.Join(runRel, "dist/index.html"), "text/html", true, nil),
		urlArtifact(bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-preview-url", "preview_url", "file://"+filepath.Join(runDir, "preview/index.html"), true),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-preview", "preview_page", filepath.Join(runRel, "preview/index.html"), "text/html", false, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-screenshot", "screenshot", filepath.Join(runRel, "screenshots/preview.png"), "image/png", true, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-validation", "validation_report", filepath.Join(runRel, "validation_report.json"), "application/json", true, nil),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-stdout", "log", filepath.Join(runRel, "build.stdout.log"), "text/plain", false, map[string]string{"stream": "stdout"}),
		fileArtifact(s.Workspace, bundle.Promotion.RunID, "artifact-"+bundle.LessonID+"-stderr", "log", filepath.Join(runRel, "build.stderr.log"), "text/plain", false, map[string]string{"stream": "stderr"}),
	}
	bundle.Artifacts = arts
	bundle.Evidence = evidence
	result.PreviewPath = filepath.Join(runDir, "preview/index.html")
	result.PreviewURL = "file://" + result.PreviewPath
	result.ValidationPath = filepath.Join(runDir, "validation_report.json")
	result.ArtifactPaths = collectPaths(arts, s.Workspace)
	return bundle, result, nil
}

func (s *Service) writeGraph(ctx context.Context, kind, lessonID, runID string, bundle LessonBundle, ts time.Time, decision string) error {
	if err := s.Store.AppendGraph(ctx, edge(ts, "skill:"+generatorName(kind), bundle.ID, "produces", runID, nil)); err != nil {
		return err
	}
	for _, a := range bundle.Artifacts {
		rel := relationFor(kind, a.Kind)
		if rel == "" {
			continue
		}
		if err := s.Store.AppendGraph(ctx, edge(ts, "run:"+runID, a.ID, rel, runID, nil)); err != nil {
			return err
		}
	}
	if err := s.Store.AppendGraph(ctx, edge(ts, "eval:"+evalGateID(kind), bundle.ID, "validates", runID, evidenceIDs(bundle.Evidence))); err != nil {
		return err
	}
	if decision != DecisionAccepted {
		for _, ev := range bundle.Evidence {
			if err := s.Store.AppendGraph(ctx, edge(ts, "run:"+runID, ev.ID, "failed_because", runID, []string{ev.ID})); err != nil {
				return err
			}
		}
	}
	_ = lessonID
	return nil
}

func relationFor(kind, artifactKind string) string {
	switch artifactKind {
	case "video":
		return "renders"
	case "log", "screenshot":
		return "captures"
	case "validation_report":
		return "validates"
	case "html_app":
		return "builds"
	case "preview_url", "preview_page":
		return "previews"
	}
	return ""
}

func generatorName(kind string) string {
	if kind == KindManimLesson {
		return "create_manim_lesson"
	}
	return "create_browser_game_lesson"
}

func evalGateID(kind string) string {
	if kind == KindManimLesson {
		return "eval:manim_lesson_smoke"
	}
	return "eval:browser_game_smoke"
}

func lineage(ts time.Time, event, lessonID, runID, gate string, artifacts, evidence []string, reason string) LineageRecord {
	return LineageRecord{
		SchemaVersion: RecordSchema,
		ID:            stableRecordID("lineage", event, runID, lessonID, ts),
		Timestamp:     ts,
		Actor:         "nanogo-meta",
		Event:         event,
		LessonID:      lessonID,
		RunID:         runID,
		EvalGateID:    gate,
		ArtifactIDs:   artifacts,
		EvidenceIDs:   evidence,
		Reason:        reason,
	}
}

func edge(ts time.Time, from, to, rel, runID string, evidence []string) GraphEdge {
	return GraphEdge{
		SchemaVersion: RecordSchema,
		ID:            stableRecordID("graph", rel, from, to, ts),
		Timestamp:     ts,
		From:          from,
		To:            to,
		Relation:      rel,
		RunID:         runID,
		EvidenceIDs:   evidence,
	}
}

func writeFiles(root string, files map[string]string) error {
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func collectPaths(artifacts []ArtifactRef, workspace string) []string {
	var paths []string
	for _, a := range artifacts {
		if a.Path != "" {
			paths = append(paths, filepath.Join(workspace, a.Path))
		}
	}
	return paths
}

func titleFor(kind, prompt string) string {
	if kind == KindManimLesson {
		return "Manim lesson: " + prompt
	}
	return "Browser game lesson: " + prompt
}

func (s *Service) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock().UTC()
}
