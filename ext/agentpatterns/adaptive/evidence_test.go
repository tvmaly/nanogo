package adaptive_test

import (
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	patternadaptive "github.com/tvmaly/nanogo/ext/agentpatterns/adaptive"
)

func TestPatternResultEvidenceCanBeArchivedWithoutPromotion(t *testing.T) {
	got := patternadaptive.FromPatternResult(contracts.PatternResult{
		Evidence: []contracts.EvidenceRecord{{Kind: "pattern_result", SubjectID: "cross", Value: map[string]any{"score": 0.8}}},
	})
	if len(got.Records) != 1 || got.Records[0].Kind != "pattern_result" {
		t.Fatalf("evidence = %#v", got)
	}
}
