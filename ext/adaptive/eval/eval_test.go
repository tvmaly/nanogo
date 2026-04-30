package eval_test

import (
	"math"
	"testing"

	"github.com/tvmaly/nanogo/ext/adaptive"
	adeval "github.com/tvmaly/nanogo/ext/adaptive/eval"
)

func TestScoringWeights(t *testing.T) {
	t.Parallel()
	r := adaptive.AdaptiveEvalResult{
		MasteryGain: 0.5, RetentionScore: 0.4, TransferScore: 0.3,
		EngagementScore: 0.2, QualityScore: 0.9, ParentRating: 1,
		FrustrationScore: 0.1, TimeToMasteryMin: 12, CostUSD: 0.03,
	}
	cfg := adeval.ScoreConfig{Mastery: 2, Retention: 3, Transfer: 4, Engagement: 5, Quality: 6, ParentRating: 7, Frustration: 8, TimePenalty: 0.5, TargetTimeMin: 10, CostPenalty: 10}
	got := adeval.Score(r, cfg).CombinedScore
	want := .5*2 + .4*3 + .3*4 + .2*5 + .9*6 + 1*7 - .1*8 - (12-10)*.5 - .03*10
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("score = %v want %v", got, want)
	}
}

func TestScoringPenalties(t *testing.T) {
	t.Parallel()
	base := adaptive.AdaptiveEvalResult{MasteryGain: 1, RetentionScore: 1, TransferScore: 1, EngagementScore: 1, QualityScore: 1, ParentRating: 1}
	low := adeval.Score(base, adeval.DefaultScoreConfig()).CombinedScore
	highPenalty := base
	highPenalty.FrustrationScore = 0.8
	highPenalty.TimeToMasteryMin = 80
	highPenalty.CostUSD = 1
	high := adeval.Score(highPenalty, adeval.DefaultScoreConfig()).CombinedScore
	if high >= low {
		t.Fatalf("penalized score %v should be lower than %v", high, low)
	}
}
