// Package cost records per-turn token costs as JSONL.
package cost

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

type Price struct {
	InputPerMTok       float64 `json:"input_per_mtok"`
	OutputPerMTok      float64 `json:"output_per_mtok"`
	CachedInputPerMTok float64 `json:"cached_input_per_mtok"`
}

type Config struct {
	OutputPath string           `json:"output_path"`
	Prices     map[string]Price `json:"prices"`
}

type Tracker struct {
	cfg Config
	mu  sync.Mutex
}

func New(cfg Config) *Tracker { return &Tracker{cfg: cfg} }

func (t *Tracker) Record(_ context.Context, e event.Event) error {
	if e.Kind != event.TurnCompleted {
		return nil
	}
	p, _ := e.Payload.(event.TurnCompletedPayload)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(t.cfg.OutputPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.cfg.OutputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	rec := map[string]any{
		"time": e.At.Format(time.RFC3339Nano), "session": e.Session, "turn": e.Turn, "model": p.Model,
		"input_tokens": p.InputTokens, "output_tokens": p.OutputTokens, "cached_input_tokens": p.CachedInputTokens,
		"source": p.Source, "skill": p.Skill,
	}
	if len(p.ServerToolUse) > 0 {
		rec["server_tool_use"] = p.ServerToolUse
	}
	if p.InputTokens == 0 && p.OutputTokens == 0 && p.CachedInputTokens == 0 {
		rec["cost_usd"], rec["error"] = nil, "no_usage"
	} else if price, ok := t.cfg.Prices[p.Model]; ok {
		uncached := p.InputTokens - p.CachedInputTokens
		rec["cost_usd"] = (float64(uncached)/1_000_000*price.InputPerMTok +
			float64(p.CachedInputTokens)/1_000_000*price.CachedInputPerMTok +
			float64(p.OutputTokens)/1_000_000*price.OutputPerMTok)
	} else {
		rec["cost_usd"], rec["error"] = nil, "unknown_model"
	}
	return json.NewEncoder(f).Encode(rec)
}

func Summary(path, by string, since time.Duration) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	type agg struct {
		in, out, cached, turns int
		usd                    float64
		unknown                bool
	}
	groups := map[string]*agg{"total": {}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r struct {
			Time              string   `json:"time"`
			Model             string   `json:"model"`
			Source            string   `json:"source"`
			Skill             string   `json:"skill"`
			Error             string   `json:"error"`
			InputTokens       int      `json:"input_tokens"`
			OutputTokens      int      `json:"output_tokens"`
			CachedInputTokens int      `json:"cached_input_tokens"`
			CostUSD           *float64 `json:"cost_usd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return "", err
		}
		if !cutoff.IsZero() {
			if ts, _ := time.Parse(time.RFC3339Nano, r.Time); ts.Before(cutoff) {
				continue
			}
		}
		key := "total"
		switch by {
		case "model":
			key = r.Model
		case "source":
			key = r.Source
		case "skill":
			key = r.Skill
		}
		if key == "" {
			key = "unknown"
		}
		for _, k := range []string{"total", key} {
			a := groups[k]
			if a == nil {
				a = &agg{}
				groups[k] = a
			}
			a.turns++
			a.in += r.InputTokens
			a.out += r.OutputTokens
			a.cached += r.CachedInputTokens
			if r.CostUSD != nil {
				a.usd += *r.CostUSD
			} else {
				a.unknown = true
			}
		}
	}
	var b strings.Builder
	for k, a := range groups {
		fmt.Fprintf(&b, "%s turns=%d input=%d output=%d cached=%d", k, a.turns, a.in, a.out, a.cached)
		if !a.unknown {
			fmt.Fprintf(&b, " usd=%.6f", a.usd)
		}
		b.WriteByte('\n')
	}
	return b.String(), sc.Err()
}
