package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tvmaly/nanogo/core/llm"
)

// --- Registry tests ---

func TestRegistry_RegisterAndBuild(t *testing.T) {
	t.Parallel()
	called := false
	llm.Register("testprovider", func(cfg json.RawMessage) (llm.Provider, error) {
		called = true
		return &fakeProvider{name: "testprovider"}, nil
	})
	p, err := llm.Build("testprovider", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if !called {
		t.Fatal("factory not called")
	}
}

func TestRegistry_UnknownDriver(t *testing.T) {
	t.Parallel()
	_, err := llm.Build("doesnotexist_xyz", nil)
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
	if !errors.Is(err, llm.ErrUnknownDriver) {
		t.Fatalf("expected ErrUnknownDriver, got %v", err)
	}
}

// --- Router tests ---

func TestRouter_DispatchBySource(t *testing.T) {
	t.Parallel()
	cheap := &fakeProvider{name: "cheap"}
	expensive := &fakeProvider{name: "expensive"}

	router := &llm.Router{
		Providers: map[string]llm.Provider{"cheap": cheap, "expensive": expensive},
		Rules: []llm.Rule{
			{When: "source=heartbeat", Route: "cheap"},
			{When: "default", Route: "expensive"},
		},
		Fallback: "expensive",
	}

	// heartbeat source → cheap
	ctx := context.WithValue(context.Background(), llm.CtxKeySource, "heartbeat")
	ch, err := router.Chat(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	drain(ch)
	if cheap.calls != 1 {
		t.Fatalf("cheap.calls = %d, want 1", cheap.calls)
	}

	// no source → expensive (default)
	ch, err = router.Chat(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	drain(ch)
	if expensive.calls != 1 {
		t.Fatalf("expensive.calls = %d, want 1", expensive.calls)
	}

	// cli source → expensive (falls through to default)
	ctx2 := context.WithValue(context.Background(), llm.CtxKeySource, "cli")
	ch, err = router.Chat(ctx2, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	drain(ch)
	if expensive.calls != 2 {
		t.Fatalf("expensive.calls = %d after cli, want 2", expensive.calls)
	}
}

func TestRouterOrdering(t *testing.T) {
	t.Parallel()
	a := &fakeProvider{name: "a"}
	b := &fakeProvider{name: "b"}

	router := &llm.Router{
		Providers: map[string]llm.Provider{"a": a, "b": b},
		Rules: []llm.Rule{
			{When: "source=cli", Route: "a"},
			{When: "source=cli", Route: "b"}, // should never match (first wins)
			{When: "default", Route: "b"},
		},
		Fallback: "b",
	}

	ctx := context.WithValue(context.Background(), llm.CtxKeySource, "cli")
	ch, err := router.Chat(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	drain(ch)
	if a.calls != 1 || b.calls != 0 {
		t.Fatalf("first-match not respected: a=%d b=%d", a.calls, b.calls)
	}
}

func TestRouterOrdering_NoMatchNoDefault(t *testing.T) {
	t.Parallel()
	router := &llm.Router{
		Providers: map[string]llm.Provider{"a": &fakeProvider{}},
		Rules:     []llm.Rule{{When: "source=cli", Route: "a"}},
		Fallback:  "",
	}
	_, err := router.Chat(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error when no rule matches and no fallback")
	}
}

func TestRouterOrdering_MissingRouteAtBuildTime(t *testing.T) {
	t.Parallel()
	router := &llm.Router{
		Providers: map[string]llm.Provider{"a": &fakeProvider{}},
		Rules:     []llm.Rule{{When: "default", Route: "missing"}},
		Fallback:  "missing",
	}
	err := router.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing route")
	}
}

func TestRouter_SkillAndSubagentMatching(t *testing.T) {
	t.Parallel()
	cheap := &fakeProvider{name: "cheap"}
	expensive := &fakeProvider{name: "expensive"}

	router := &llm.Router{
		Providers: map[string]llm.Provider{"cheap": cheap, "expensive": expensive},
		Rules: []llm.Rule{
			{When: "skill=consolidator", Route: "cheap"},
			{When: "subagent=true", Route: "cheap"},
			{When: "default", Route: "expensive"},
		},
		Fallback: "expensive",
	}

	// skill=consolidator
	ctx := context.WithValue(context.Background(), llm.CtxKeySkill, "consolidator")
	ch, _ := router.Chat(ctx, llm.Request{})
	drain(ch)
	if cheap.calls != 1 {
		t.Fatalf("skill match: cheap.calls=%d want 1", cheap.calls)
	}

	// subagent=true
	ctx2 := context.WithValue(context.Background(), llm.CtxKeySubagent, true)
	ch, _ = router.Chat(ctx2, llm.Request{})
	drain(ch)
	if cheap.calls != 2 {
		t.Fatalf("subagent match: cheap.calls=%d want 2", cheap.calls)
	}

	// non-matching skill falls through
	ctx3 := context.WithValue(context.Background(), llm.CtxKeySkill, "other")
	ch, _ = router.Chat(ctx3, llm.Request{})
	drain(ch)
	if expensive.calls != 1 {
		t.Fatalf("fallthrough: expensive.calls=%d want 1", expensive.calls)
	}
}

// --- helpers ---

type fakeProvider struct {
	name  string
	calls int
}

func (f *fakeProvider) Chat(_ context.Context, _ llm.Request) (<-chan llm.Chunk, error) {
	f.calls++
	ch := make(chan llm.Chunk, 1)
	ch <- llm.Chunk{TextDelta: "ok", FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func drain(ch <-chan llm.Chunk) {
	for range ch {
	}
}
