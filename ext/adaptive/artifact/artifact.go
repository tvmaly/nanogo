// Package artifact re-exports adaptive artifact data types for JSON manifests.
package artifact

import "github.com/tvmaly/nanogo/ext/adaptive"

type ArtifactKind = adaptive.ArtifactKind
type AdaptiveArtifact = adaptive.AdaptiveArtifact
type Attempt = adaptive.Attempt
type AdaptiveEvalResult = adaptive.AdaptiveEvalResult
type MutationGoal = adaptive.MutationGoal
type ExperimentPlan = adaptive.ExperimentPlan

const (
	ArtifactLessonBundle = adaptive.ArtifactLessonBundle
	ArtifactPathway      = adaptive.ArtifactPathway
	ArtifactTutorPolicy  = adaptive.ArtifactTutorPolicy
	ArtifactRubric       = adaptive.ArtifactRubric
	ArtifactPrompt       = adaptive.ArtifactPrompt
	ArtifactSkill        = adaptive.ArtifactSkill
	ArtifactTemplate     = adaptive.ArtifactTemplate
)
