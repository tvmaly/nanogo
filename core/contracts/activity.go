package contracts

import "context"

type ActivityObserver interface {
	ObserveActivity(context.Context, ActivityObservationRequest) (ActivityObservation, error)
}

type ActivityObservationRequest struct {
	SchemaVersion, SessionID, ChildID, LessonID, MicroLessonID, AttemptID, RubricID string
	FrameRefs                                                                       []ArtifactRef
	MaxCostUSD                                                                      float64
	Metadata                                                                        map[string]string
}

type ActivityObservation struct {
	SchemaVersion, Observer, RawRef string
	Checks                          []ObservationCheck
	CostUSD                         float64
	Metadata                        map[string]string
}

type ObservationCheck struct {
	ID, Evidence string
	Present      bool
	Confidence   float64
	Critical     bool
}
