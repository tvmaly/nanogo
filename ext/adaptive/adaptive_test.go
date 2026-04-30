package adaptive_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/ext/adaptive/archive"
)

type testDomain struct{}

func (testDomain) Name() string { return "fake-registry" }
func (testDomain) ArtifactKinds() []adaptive.ArtifactKind {
	return []adaptive.ArtifactKind{adaptive.ArtifactPrompt}
}
func (testDomain) Compile(context.Context, adaptive.CompileRequest) ([]adaptive.AdaptiveArtifact, error) {
	return nil, nil
}
func (testDomain) Evaluate(context.Context, adaptive.AdaptiveArtifact, adaptive.Attempt) (adaptive.AdaptiveEvalResult, error) {
	return adaptive.AdaptiveEvalResult{}, nil
}
func (testDomain) Mutate(context.Context, adaptive.AdaptiveArtifact, adaptive.MutationGoal) ([]adaptive.AdaptiveArtifact, error) {
	return nil, nil
}

func TestDomainRegistry(t *testing.T) {
	name := "fake-registry"
	adaptive.RegisterDomain(name, func(json.RawMessage) (adaptive.DomainAdapter, error) {
		return testDomain{}, nil
	})
	got, err := adaptive.BuildDomain(name, nil)
	if err != nil {
		t.Fatalf("BuildDomain: %v", err)
	}
	if got.Name() != name {
		t.Fatalf("domain name = %q", got.Name())
	}
	if _, err := adaptive.BuildDomain("missing-domain", nil); err == nil || !strings.Contains(err.Error(), "missing-domain") {
		t.Fatalf("unknown domain error = %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate registration should panic")
		}
	}()
	adaptive.RegisterDomain(name, func(json.RawMessage) (adaptive.DomainAdapter, error) {
		return testDomain{}, nil
	})
}

func TestFakeDomainEndToEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	ar, err := archive.New(root)
	if err != nil {
		t.Fatalf("archive.New: %v", err)
	}
	d := adaptive.FakeDomain{}
	arts, err := d.Compile(ctx, adaptive.CompileRequest{
		ChildID: "cross", Subject: "science", Topic: "magnets", SourceBody: "magnet idea",
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(arts) < 2 {
		t.Fatalf("want at least 2 artifacts, got %d", len(arts))
	}
	for _, a := range arts {
		if err := ar.AddArtifact(ctx, a); err != nil {
			t.Fatalf("AddArtifact: %v", err)
		}
	}
	out, err := d.Evaluate(ctx, arts[0], adaptive.Attempt{
		ID: "attempt-1", ArtifactID: arts[0].ID, ChildID: "cross", StartedAt: time.Now(),
		Observations: map[string]any{"score": 0.8},
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if err := ar.AddOutcome(ctx, out); err != nil {
		t.Fatalf("AddOutcome: %v", err)
	}
	mutated, err := d.Mutate(ctx, arts[0], adaptive.MutationGoal{ChildID: "cross", Improve: []string{"retention"}})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if len(mutated) == 0 || mutated[0].ParentID != arts[0].ID || mutated[0].Version != arts[0].Version+1 {
		t.Fatalf("bad mutation: %+v", mutated)
	}
	if err := ar.AddArtifact(ctx, mutated[0]); err != nil {
		t.Fatalf("AddArtifact mutated: %v", err)
	}
}
