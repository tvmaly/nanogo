package manual_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	"github.com/tvmaly/nanogo/ext/eval/manual"
)

func TestManualObserverReturnsConfiguredChecks(t *testing.T) {
	got, err := (manual.Observer{Checks: []contracts.ObservationCheck{{ID: "full-extension", Present: true}}}).ObserveActivity(context.Background(), contracts.ActivityObservationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Observer != "manual" || len(got.Checks) != 1 || !got.Checks[0].Present {
		t.Fatalf("observation = %+v", got)
	}
}
