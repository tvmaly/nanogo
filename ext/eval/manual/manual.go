package manual

import (
	"context"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Observer struct {
	Checks []contracts.ObservationCheck
}

func (o Observer) ObserveActivity(context.Context, contracts.ActivityObservationRequest) (contracts.ActivityObservation, error) {
	return contracts.ActivityObservation{SchemaVersion: "activity.observation.v1", Observer: "manual", Checks: append([]contracts.ObservationCheck(nil), o.Checks...)}, nil
}
