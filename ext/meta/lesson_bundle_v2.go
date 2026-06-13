package meta

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	LessonBundleSchemaV2 = "meta.lesson_bundle.v2"
	KindBrowserMicro     = "browser_micro_lesson"
	RubricSchemaV1       = "eval.rubric.v1"
)

type ObjectiveType string

const (
	ObjectiveSurface       ObjectiveType = "surface"
	ObjectiveDeep          ObjectiveType = "deep"
	ObjectiveTransfer      ObjectiveType = "transfer"
	ObjectivePhysicalSkill ObjectiveType = "physical_skill"
	ObjectiveMixed         ObjectiveType = "mixed"
)

type EvidenceKind string

const (
	EvidencePhysicalPerformance EvidenceKind = "physical_performance"
	EvidenceStudentReasoning    EvidenceKind = "student_reasoning"
	EvidenceReflection          EvidenceKind = "reflection"
	EvidenceTransfer            EvidenceKind = "transfer"
	EvidenceParentConfirmation  EvidenceKind = "parent_confirmation"
)

type CompletionRule string

const (
	CompletionPhysicalPass               CompletionRule = "physical_pass"
	CompletionPhysicalPassPlusReflection CompletionRule = "physical_pass_plus_reflection"
	CompletionAllRequiredEvidence        CompletionRule = "all_required_evidence"
	CompletionParentConfirmed            CompletionRule = "parent_confirmed"
)

type LessonBundleV2 struct {
	SchemaVersion string          `json:"schema_version" yaml:"schema_version"`
	Kind          string          `json:"kind" yaml:"kind"`
	ID            string          `json:"id" yaml:"id"`
	Title         string          `json:"title" yaml:"title"`
	Dashboard     DashboardRef    `json:"dashboard" yaml:"dashboard"`
	MicroLessons  []MicroLessonV2 `json:"micro_lessons" yaml:"micro_lessons"`
}

type DashboardRef struct {
	FamilyID string `json:"family_id,omitempty" yaml:"family_id,omitempty"`
	ChildID  string `json:"child_id" yaml:"child_id"`
}

type MicroLessonV2 struct {
	ID                string           `json:"id" yaml:"id"`
	Title             string           `json:"title" yaml:"title"`
	Concept           string           `json:"concept" yaml:"concept"`
	ObjectiveType     ObjectiveType    `json:"objective_type" yaml:"objective_type"`
	StudentWorkTarget string           `json:"student_work_target" yaml:"student_work_target"`
	Requires          []string         `json:"requires" yaml:"requires"`
	Video             VideoSegment     `json:"video" yaml:"video"`
	SafetySetup       SafetySetup      `json:"safety_setup,omitempty" yaml:"safety_setup,omitempty"`
	TutorFlow         TutorFlow        `json:"tutor_flow" yaml:"tutor_flow"`
	Activity          ActivitySpec     `json:"activity" yaml:"activity"`
	Evaluation        EvaluationSpec   `json:"evaluation" yaml:"evaluation"`
	LearningEvidence  LearningEvidence `json:"learning_evidence" yaml:"learning_evidence"`
	Advancement       AdvancementRule  `json:"advancement" yaml:"advancement"`
}

type VideoSegment struct {
	Provider            string `json:"provider" yaml:"provider"`
	VideoID             string `json:"video_id" yaml:"video_id"`
	StartSeconds        int    `json:"start_seconds" yaml:"start_seconds"`
	EndSeconds          int    `json:"end_seconds" yaml:"end_seconds"`
	Provenance          string `json:"provenance" yaml:"provenance"`
	SelectedBecause     string `json:"selected_because" yaml:"selected_because"`
	ParentCheckRequired bool   `json:"parent_check_required" yaml:"parent_check_required"`
}

type SafetySetup struct {
	Child  string `json:"child" yaml:"child"`
	Parent string `json:"parent" yaml:"parent"`
}

type TutorFlow struct {
	OpeningPrompt    string          `json:"opening_prompt" yaml:"opening_prompt"`
	DiagnosticPrompt string          `json:"diagnostic_prompt" yaml:"diagnostic_prompt"`
	ScaffoldLadder   []ScaffoldStep  `json:"scaffold_ladder" yaml:"scaffold_ladder"`
	ExplanationCard  ExplanationCard `json:"explanation_card" yaml:"explanation_card"`
	FadeRule         string          `json:"fade_rule,omitempty" yaml:"fade_rule,omitempty"`
}

type ScaffoldStep struct {
	Level  int    `json:"level" yaml:"level"`
	Move   string `json:"move" yaml:"move"`
	Prompt string `json:"prompt" yaml:"prompt"`
}

type ExplanationCard struct {
	UseAfter    []string `json:"use_after" yaml:"use_after"`
	Concise     string   `json:"concise" yaml:"concise"`
	ActiveCheck string   `json:"active_check" yaml:"active_check"`
}

type ActivitySpec struct {
	Instructions      string `json:"instructions" yaml:"instructions"`
	SuccessCriterion  string `json:"success_criterion" yaml:"success_criterion"`
	Capture           string `json:"capture" yaml:"capture"`
	MaxCaptureSeconds int    `json:"max_capture_seconds" yaml:"max_capture_seconds"`
}

type EvaluationSpec struct {
	RubricID string       `json:"rubric_id" yaml:"rubric_id"`
	Sampling SamplingSpec `json:"sampling,omitempty" yaml:"sampling,omitempty"`
}

type SamplingSpec struct {
	FPS       int `json:"fps,omitempty" yaml:"fps,omitempty"`
	MaxFrames int `json:"max_frames,omitempty" yaml:"max_frames,omitempty"`
}

type LearningEvidence struct {
	Requires         []EvidenceKind `json:"requires" yaml:"requires"`
	CompletionRule   CompletionRule `json:"completion_rule" yaml:"completion_rule"`
	ReasoningPrompt  string         `json:"reasoning_prompt,omitempty" yaml:"reasoning_prompt,omitempty"`
	ReflectionPrompt string         `json:"reflection_prompt,omitempty" yaml:"reflection_prompt,omitempty"`
	TransferPrompt   string         `json:"transfer_prompt,omitempty" yaml:"transfer_prompt,omitempty"`
}

type AdvancementRule struct {
	Mode             string  `json:"mode" yaml:"mode"`
	MasteryThreshold float64 `json:"mastery_threshold" yaml:"mastery_threshold"`
}

type RubricV1 struct {
	SchemaVersion string         `json:"schema_version" yaml:"schema_version"`
	ID            string         `json:"id" yaml:"id"`
	MicroLesson   string         `json:"micro_lesson" yaml:"micro_lesson"`
	PassRule      CompletionRule `json:"pass_rule" yaml:"pass_rule"`
	Checks        []RubricCheck  `json:"checks" yaml:"checks"`
}

type RubricCheck struct {
	ID          string `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required" yaml:"required"`
	Critical    bool   `json:"critical,omitempty" yaml:"critical,omitempty"`
}

func LoadLessonBundleV2(path string) (LessonBundleV2, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LessonBundleV2{}, err
	}
	var header struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	_ = yaml.Unmarshal(data, &header)
	if header.SchemaVersion == LessonBundleSchemaV2 {
		var b LessonBundleV2
		if err := yaml.Unmarshal(data, &b); err != nil {
			return LessonBundleV2{}, err
		}
		return b, ValidateLessonBundleV2(b)
	}
	var v1 LessonBundle
	if err := yaml.Unmarshal(data, &v1); err != nil {
		return LessonBundleV2{}, err
	}
	return NormalizeLessonBundleV1(v1), nil
}

func NormalizeLessonBundleV1(v1 LessonBundle) LessonBundleV2 {
	title := v1.Title
	if title == "" {
		title = v1.LessonID
	}
	return LessonBundleV2{
		SchemaVersion: LessonBundleSchemaV2,
		Kind:          KindBrowserMicro,
		ID:            nonemptyMeta(v1.LessonID, v1.ID),
		Title:         title,
		MicroLessons: []MicroLessonV2{{
			ID:                "ml-01",
			Title:             title,
			Concept:           strings.Join(v1.LearningObjectives, "; "),
			ObjectiveType:     ObjectiveSurface,
			StudentWorkTarget: title,
			LearningEvidence:  LearningEvidence{Requires: []EvidenceKind{EvidenceStudentReasoning}, CompletionRule: CompletionAllRequiredEvidence},
			Advancement:       AdvancementRule{Mode: "linear", MasteryThreshold: 0.8},
		}},
	}
}

func ValidateLessonBundleV2(b LessonBundleV2) error {
	var problems []string
	if b.SchemaVersion != LessonBundleSchemaV2 {
		problems = append(problems, "schema_version must be "+LessonBundleSchemaV2)
	}
	if b.Kind != KindBrowserMicro {
		problems = append(problems, "kind must be "+KindBrowserMicro)
	}
	if len(b.MicroLessons) == 0 {
		problems = append(problems, "micro_lessons requires at least one item")
	}
	ids := map[string]bool{}
	for i, ml := range b.MicroLessons {
		prefix := fmt.Sprintf("micro_lessons[%d]", i)
		if ml.ID == "" {
			problems = append(problems, prefix+".id is required")
		}
		ids[ml.ID] = true
		if !validObjective(ml.ObjectiveType) {
			problems = append(problems, prefix+".objective_type is unsupported")
		}
		if ml.StudentWorkTarget == "" {
			problems = append(problems, prefix+".student_work_target is required")
		}
		if err := validateEvidenceRule(ml.ObjectiveType, ml.LearningEvidence); err != nil {
			problems = append(problems, prefix+".learning_evidence: "+err.Error())
		}
		if ml.Activity.Capture != "" && ml.Activity.Capture != "none" && ml.Evaluation.RubricID == "" {
			problems = append(problems, prefix+".evaluation.rubric_id is required when capture is enabled")
		}
		if ml.ObjectiveType == ObjectivePhysicalSkill || ml.ObjectiveType == ObjectiveMixed {
			if ml.SafetySetup.Child == "" || ml.SafetySetup.Parent == "" {
				problems = append(problems, prefix+".safety_setup child and parent are required")
			}
		}
		if ml.TutorFlow.OpeningPrompt == "" || ml.TutorFlow.DiagnosticPrompt == "" {
			problems = append(problems, prefix+".tutor_flow opening_prompt and diagnostic_prompt are required")
		}
		if len(ml.TutorFlow.ScaffoldLadder) < 2 {
			problems = append(problems, prefix+".tutor_flow.scaffold_ladder requires at least two entries")
		}
		for _, dep := range ml.Requires {
			if dep == ml.ID {
				problems = append(problems, prefix+".requires cannot include self")
			}
		}
	}
	problems = append(problems, validateDAG(b.MicroLessons, ids)...)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ValidateRubricV1(r RubricV1) error {
	var problems []string
	if r.SchemaVersion != RubricSchemaV1 {
		problems = append(problems, "schema_version must be "+RubricSchemaV1)
	}
	if r.ID == "" {
		problems = append(problems, "id is required")
	}
	if !validCompletionRule(r.PassRule) {
		problems = append(problems, "pass_rule is outside the closed enum")
	}
	if len(r.Checks) == 0 {
		problems = append(problems, "checks requires at least one item")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateEvidenceRule(obj ObjectiveType, ev LearningEvidence) error {
	if len(ev.Requires) == 0 {
		return errors.New("requires is required")
	}
	if !validCompletionRule(ev.CompletionRule) {
		return errors.New("completion_rule is unsupported")
	}
	for _, r := range ev.Requires {
		if !validEvidence(r) {
			return fmt.Errorf("requires contains unsupported evidence %q", r)
		}
	}
	physicalOnly := len(ev.Requires) == 1 && ev.Requires[0] == EvidencePhysicalPerformance
	if obj == ObjectivePhysicalSkill && physicalOnly && ev.CompletionRule != CompletionPhysicalPass {
		return errors.New("physical_skill with only physical_performance must use physical_pass")
	}
	if (obj == ObjectiveDeep || obj == ObjectiveTransfer || obj == ObjectiveMixed) && physicalOnly {
		return errors.New("deep, transfer, and mixed objectives need configured evidence beyond visual pass")
	}
	return nil
}

func validateDAG(lessons []MicroLessonV2, ids map[string]bool) []string {
	var problems []string
	for _, ml := range lessons {
		for _, dep := range ml.Requires {
			if !ids[dep] {
				problems = append(problems, fmt.Sprintf("micro_lesson %s requires unknown %s", ml.ID, dep))
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	byID := map[string]MicroLessonV2{}
	for _, ml := range lessons {
		byID[ml.ID] = ml
	}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			problems = append(problems, "prerequisite cycle includes "+id)
			return false
		}
		if visited[id] {
			return true
		}
		visiting[id] = true
		for _, dep := range byID[id].Requires {
			if ids[dep] {
				visit(dep)
			}
		}
		visiting[id] = false
		visited[id] = true
		return true
	}
	for id := range byID {
		visit(id)
	}
	return problems
}

func validObjective(v ObjectiveType) bool {
	switch v {
	case ObjectiveSurface, ObjectiveDeep, ObjectiveTransfer, ObjectivePhysicalSkill, ObjectiveMixed:
		return true
	default:
		return false
	}
}

func validEvidence(v EvidenceKind) bool {
	switch v {
	case EvidencePhysicalPerformance, EvidenceStudentReasoning, EvidenceReflection, EvidenceTransfer, EvidenceParentConfirmation:
		return true
	default:
		return false
	}
}

func validCompletionRule(v CompletionRule) bool {
	switch v {
	case CompletionPhysicalPass, CompletionPhysicalPassPlusReflection, CompletionAllRequiredEvidence, CompletionParentConfirmed:
		return true
	default:
		return false
	}
}

func nonemptyMeta(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
