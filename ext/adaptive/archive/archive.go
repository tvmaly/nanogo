// Package archive stores adaptive artifacts and outcomes in JSONL files.
package archive

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive"
)

type Query struct {
	ChildID         string
	Subject         string
	Topic           string
	Strategy        string
	IncludeFailures bool
}

type Archive struct {
	root string
	dir  string
	mu   sync.Mutex
}

func New(root string) (*Archive, error) {
	dir := filepath.Join(root, "memory", "adaptive")
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0755); err != nil {
		return nil, err
	}
	return &Archive{root: root, dir: dir}, nil
}

func (a *Archive) Root() string { return a.root }

func (a *Archive) AddArtifact(_ context.Context, art adaptive.AdaptiveArtifact) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if art.SchemaVersion == "" {
		art.SchemaVersion = "adaptive.artifact.v1"
	}
	if art.CreatedAt.IsZero() {
		art.CreatedAt = time.Now().UTC()
	}
	return appendJSON(filepath.Join(a.dir, "archive.jsonl"), art)
}

func (a *Archive) AddOutcome(_ context.Context, r adaptive.AdaptiveEvalResult) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.SchemaVersion == "" {
		r.SchemaVersion = "adaptive.outcome.v1"
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	return appendJSON(filepath.Join(a.dir, "outcomes.jsonl"), r)
}

func (a *Archive) Artifacts(context.Context) ([]adaptive.AdaptiveArtifact, error) {
	return readJSONL[adaptive.AdaptiveArtifact](filepath.Join(a.dir, "archive.jsonl"))
}

func (a *Archive) Outcomes(context.Context) ([]adaptive.AdaptiveEvalResult, error) {
	return readJSONL[adaptive.AdaptiveEvalResult](filepath.Join(a.dir, "outcomes.jsonl"))
}

func (a *Archive) Top(ctx context.Context, q Query, n int) ([]adaptive.AdaptiveArtifact, error) {
	arts, err := a.Artifacts(ctx)
	if err != nil {
		return nil, err
	}
	outs, err := a.Outcomes(ctx)
	if err != nil {
		return nil, err
	}
	best := latestOutcomes(outs)
	var matches []adaptive.AdaptiveArtifact
	for _, art := range arts {
		if !match(art, q) {
			continue
		}
		out, ok := best[art.ID]
		if !ok {
			continue
		}
		if !q.IncludeFailures && !out.Correctness {
			continue
		}
		matches = append(matches, art)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		oi, oj := best[matches[i].ID], best[matches[j].ID]
		if oi.CombinedScore == oj.CombinedScore {
			return oi.CreatedAt.After(oj.CreatedAt)
		}
		return oi.CombinedScore > oj.CombinedScore
	})
	if n > 0 && len(matches) > n {
		matches = matches[:n]
	}
	return matches, nil
}

func (a *Archive) Similar(ctx context.Context, art adaptive.AdaptiveArtifact, n int) ([]adaptive.AdaptiveArtifact, error) {
	arts, err := a.Artifacts(ctx)
	if err != nil {
		return nil, err
	}
	var out []adaptive.AdaptiveArtifact
	for _, candidate := range arts {
		if art.Subject != "" && candidate.Subject != art.Subject {
			continue
		}
		if art.Topic != "" && candidate.Topic != art.Topic {
			continue
		}
		out = append(out, candidate)
	}
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (a *Archive) Lineage(ctx context.Context, id string) ([]adaptive.AdaptiveArtifact, error) {
	arts, err := a.Artifacts(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]adaptive.AdaptiveArtifact{}
	for _, art := range arts {
		byID[art.ID] = art
	}
	var out []adaptive.AdaptiveArtifact
	for id != "" {
		art, ok := byID[id]
		if !ok {
			if len(out) > 0 {
				if out[len(out)-1].Metadata == nil {
					out[len(out)-1].Metadata = map[string]any{}
				}
				out[len(out)-1].Metadata["lineage_warning"] = "missing parent " + id
			}
			break
		}
		out = append(out, art)
		id = art.ParentID
	}
	return out, nil
}

func latestOutcomes(outs []adaptive.AdaptiveEvalResult) map[string]adaptive.AdaptiveEvalResult {
	latest := map[string]adaptive.AdaptiveEvalResult{}
	for _, out := range outs {
		prev, ok := latest[out.ArtifactID]
		if !ok || out.CreatedAt.After(prev.CreatedAt) {
			latest[out.ArtifactID] = out
		}
	}
	return latest
}

func match(a adaptive.AdaptiveArtifact, q Query) bool {
	return (q.ChildID == "" || a.ChildID == q.ChildID) &&
		(q.Subject == "" || a.Subject == q.Subject) &&
		(q.Topic == "" || a.Topic == q.Topic) &&
		(q.Strategy == "" || a.Strategy == q.Strategy)
}

func appendJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		if sc.Text() == "" {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return out, fmt.Errorf("%s line %d: malformed JSON: %w", path, line, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}
