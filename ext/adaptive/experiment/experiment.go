// Package experiment assigns adaptive candidates to islands and selects winners.
package experiment

import (
	"math/rand"
	"strings"

	"github.com/tvmaly/nanogo/ext/adaptive"
)

var DefaultIslands = []string{"visual", "hands_on", "socratic", "story_project", "practice_retrieval"}

type IslandManager struct {
	islands []string
	next    int
}

func NewIslandManager(islands []string) *IslandManager {
	if len(islands) == 0 {
		islands = DefaultIslands
	}
	return &IslandManager{islands: append([]string(nil), islands...)}
}

func (m *IslandManager) Assign(a adaptive.AdaptiveArtifact) adaptive.AdaptiveArtifact {
	strategy := strings.ToLower(a.Strategy)
	for _, island := range m.islands {
		if strategy == island || strings.Contains(strategy, island) {
			a.IslandID = island
			return a
		}
	}
	a.IslandID = m.islands[m.next%len(m.islands)]
	m.next++
	return a
}

func Migrate(a adaptive.AdaptiveArtifact, to, reason string) adaptive.AdaptiveArtifact {
	if a.Metadata == nil {
		a.Metadata = map[string]any{}
	}
	a.Metadata["migrated_to"] = to
	a.Metadata["migration_reason"] = reason
	return a
}

type Candidate struct {
	Artifact adaptive.AdaptiveArtifact
	Score    float64
}

type SelectOptions struct {
	Mode    string
	Key     string
	Epsilon float64
}

type RandFunc func() float64

type Selector struct {
	rand  RandFunc
	round map[string]int
}

func NewSelector(r RandFunc) *Selector {
	if r == nil {
		r = rand.Float64
	}
	return &Selector{rand: r, round: map[string]int{}}
}

func (s *Selector) Select(c []Candidate, opts SelectOptions) adaptive.AdaptiveArtifact {
	if len(c) == 0 {
		return adaptive.AdaptiveArtifact{}
	}
	switch opts.Mode {
	case "round_robin":
		key := opts.Key
		i := s.round[key] % len(c)
		s.round[key]++
		return c[i].Artifact
	case "epsilon_greedy":
		if s.rand() < opts.Epsilon {
			return c[int(s.rand()*float64(len(c)))%len(c)].Artifact
		}
		return best(c)
	case "weighted":
		total := 0.0
		for _, x := range c {
			if x.Score > 0 {
				total += x.Score
			}
		}
		if total <= 0 {
			return c[0].Artifact
		}
		pick := s.rand() * total
		for _, x := range c {
			if x.Score <= 0 {
				continue
			}
			pick -= x.Score
			if pick <= 0 {
				return x.Artifact
			}
		}
		return c[len(c)-1].Artifact
	default:
		return best(c)
	}
}

func best(c []Candidate) adaptive.AdaptiveArtifact {
	best := c[0]
	for _, x := range c[1:] {
		if x.Score > best.Score {
			best = x
		}
	}
	return best.Artifact
}
