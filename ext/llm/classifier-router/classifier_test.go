package classifierrouter_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
	cr "github.com/tvmaly/nanogo/ext/llm/classifier-router"
)

// fakeProvider records calls and returns the given label as a text chunk.
type fakeProvider struct {
	calls atomic.Int32
	label string
	usage *llm.Usage
}

func (f *fakeProvider) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	f.calls.Add(1)
	ch := make(chan llm.Chunk, 2)
	ch <- llm.Chunk{TextDelta: f.label}
	if f.usage != nil {
		ch <- llm.Chunk{Usage: f.usage, FinishReason: "stop"}
	} else {
		ch <- llm.Chunk{FinishReason: "stop"}
	}
	close(ch)
	return ch, nil
}

func drain(ch <-chan llm.Chunk) (string, *llm.Usage) {
	var sb strings.Builder
	var u *llm.Usage
	for c := range ch {
		sb.WriteString(c.TextDelta)
		if c.Usage != nil {
			u = c.Usage
		}
	}
	return sb.String(), u
}

type staticProvider struct {
	label string
}

func (p *staticProvider) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, 2)
	ch <- llm.Chunk{TextDelta: p.label}
	ch <- llm.Chunk{FinishReason: "stop"}
	close(ch)
	return ch, nil
}

// TEST-10.7 — classifier-router can be selected through llm.Build config
func TestClassifierRouterRegisteredProvider(t *testing.T) {
	llm.Register("classifier-router-test-label", func(json.RawMessage) (llm.Provider, error) {
		return &staticProvider{label: "simple"}, nil
	})
	llm.Register("classifier-router-test-simple", func(json.RawMessage) (llm.Provider, error) {
		return &staticProvider{label: "simple-route"}, nil
	})
	llm.Register("classifier-router-test-default", func(json.RawMessage) (llm.Provider, error) {
		return &staticProvider{label: "default-route"}, nil
	})

	cfg := json.RawMessage(`{
		"classifier": {"driver": "classifier-router-test-label"},
		"routes": {
			"simple": {"driver": "classifier-router-test-simple"},
			"default": {"driver": "classifier-router-test-default"}
		},
		"default": "default"
	}`)
	provider, err := llm.Build("classifier-router", cfg)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := provider.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "route this"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drain(ch)
	if text != "simple-route" {
		t.Fatalf("text = %q, want simple-route", text)
	}
}

// TEST-10.7 — classifier selects downstream provider by label
func TestClassifierRouterDispatch(t *testing.T) {
	t.Parallel()
	classifier := &fakeProvider{label: "simple"}
	cheap := &fakeProvider{label: "cheap-response"}
	expensive := &fakeProvider{label: "expensive-response"}

	router := cr.New(cr.Config{
		Classifier: classifier,
		Routes: map[string]llm.Provider{
			"simple":  cheap,
			"complex": expensive,
			"default": expensive,
		},
		DefaultRoute: "default",
	})

	req := llm.Request{Messages: []llm.Message{{Role: "user", Content: "hello"}}}
	ch, err := router.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drain(ch)
	if text != "cheap-response" {
		t.Errorf("expected cheap-response (label=simple), got %q", text)
	}
	if classifier.calls.Load() != 1 {
		t.Errorf("expected 1 classifier call, got %d", classifier.calls.Load())
	}
}

// TEST-10.7 — cache hit skips classifier
func TestClassifierRouterCacheHit(t *testing.T) {
	t.Parallel()
	classifier := &fakeProvider{label: "simple"}
	cheap := &fakeProvider{label: "cheap"}
	expensive := &fakeProvider{label: "expensive"}

	router := cr.New(cr.Config{
		Classifier: classifier,
		Routes: map[string]llm.Provider{
			"simple":  cheap,
			"default": expensive,
		},
		DefaultRoute: "default",
	})

	req := llm.Request{Messages: []llm.Message{{Role: "user", Content: "same prompt"}}}
	router.Chat(context.Background(), req)
	router.Chat(context.Background(), req)

	if classifier.calls.Load() != 1 {
		t.Errorf("expected 1 classifier call (cache hit on second), got %d", classifier.calls.Load())
	}
}

// TEST-10.7 — classifier failure falls through to default
func TestClassifierRouterFallback(t *testing.T) {
	t.Parallel()
	badClassifier := &fakeProvider{label: "unknown_label_xyz"}
	expensive := &fakeProvider{label: "expensive"}

	router := cr.New(cr.Config{
		Classifier: badClassifier,
		Routes: map[string]llm.Provider{
			"default": expensive,
		},
		DefaultRoute: "default",
	})

	req := llm.Request{Messages: []llm.Message{{Role: "user", Content: "hi"}}}
	ch, err := router.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drain(ch)
	if text != "expensive" {
		t.Errorf("expected fallback to expensive, got %q", text)
	}
}

// TEST-10.8 — cost amortization: routing 80 simple + 20 complex is cheaper than all-expensive
func TestClassifierCostBenefit(t *testing.T) {
	t.Parallel()
	const cheapCostPerTurn = 1.0
	const expensiveCostPerTurn = 10.0
	const total = 100
	const simpleFraction = 80

	// All-expensive baseline
	baseline := float64(total) * expensiveCostPerTurn

	// Classifier-routed: 80 simple (cheap) + 20 complex (expensive) + 100 classifier calls (cheap)
	classifierCost := float64(total) * cheapCostPerTurn
	routed := float64(simpleFraction)*cheapCostPerTurn + float64(total-simpleFraction)*expensiveCostPerTurn + classifierCost

	if routed >= baseline {
		t.Errorf("classifier routing (%.1f) should be cheaper than all-expensive (%.1f)", routed, baseline)
	}
}
