// Package eval scores adaptive educational outcomes.
package eval

import "github.com/tvmaly/nanogo/ext/adaptive"

type ScoreConfig struct {
	Mastery       float64 `json:"mastery"`
	Retention     float64 `json:"retention"`
	Transfer      float64 `json:"transfer"`
	Engagement    float64 `json:"engagement"`
	Quality       float64 `json:"quality"`
	ParentRating  float64 `json:"parent_rating"`
	Frustration   float64 `json:"frustration"`
	TimePenalty   float64 `json:"time_penalty"`
	TargetTimeMin float64 `json:"target_time_min"`
	CostPenalty   float64 `json:"cost_penalty"`
}

func DefaultScoreConfig() ScoreConfig {
	return ScoreConfig{
		Mastery: 1, Retention: 1, Transfer: 1, Engagement: 1, Quality: 1,
		ParentRating: 1, Frustration: 1, TimePenalty: 0.02, TargetTimeMin: 30,
		CostPenalty: 1,
	}
}

func Score(r adaptive.AdaptiveEvalResult, cfg ScoreConfig) adaptive.AdaptiveEvalResult {
	if cfg == (ScoreConfig{}) {
		cfg = DefaultScoreConfig()
	}
	timeOver := r.TimeToMasteryMin - cfg.TargetTimeMin
	if timeOver < 0 {
		timeOver = 0
	}
	r.CombinedScore =
		r.MasteryGain*cfg.Mastery +
			r.RetentionScore*cfg.Retention +
			r.TransferScore*cfg.Transfer +
			r.EngagementScore*cfg.Engagement +
			r.QualityScore*cfg.Quality +
			r.ParentRating*cfg.ParentRating -
			r.FrustrationScore*cfg.Frustration -
			timeOver*cfg.TimePenalty -
			r.CostUSD*cfg.CostPenalty
	return r
}
