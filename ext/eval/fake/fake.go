package fake

import (
	"context"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Observer struct {
	Observation contracts.ActivityObservation
	Err         error
}

func NewPassObserver(checks ...string) Observer {
	obs := contracts.ActivityObservation{SchemaVersion: "activity.observation.v1", Observer: "fake"}
	for _, id := range checks {
		obs.Checks = append(obs.Checks, contracts.ObservationCheck{ID: id, Present: true, Confidence: 1, Critical: true})
	}
	return Observer{Observation: obs}
}

func (o Observer) ObserveActivity(context.Context, contracts.ActivityObservationRequest) (contracts.ActivityObservation, error) {
	if o.Observation.SchemaVersion == "" {
		o.Observation.SchemaVersion = "activity.observation.v1"
	}
	if o.Observation.Observer == "" {
		o.Observation.Observer = "fake"
	}
	return o.Observation, o.Err
}
