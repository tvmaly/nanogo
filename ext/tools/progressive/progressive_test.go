package progressive_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	coretools "github.com/tvmaly/nanogo/core/tools"
	faketools "github.com/tvmaly/nanogo/core/tools/fake"
	"github.com/tvmaly/nanogo/ext/tools/progressive"
)

func TestProgressiveSourceImplementsCoreSource(t *testing.T) {
	t.Parallel()
	var _ coretools.Source = newTestSource(t)
}

func TestProgressiveDefaultVisibleTools(t *testing.T) {
	t.Parallel()
	ps := newTestSource(t)
	list, err := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	got := names(list)
	want := []string{"read_file", "tool_help", "tool_list", "tool_reveal"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tools = %v, want %v", got, want)
	}
}

func TestProgressiveHidesContractTools(t *testing.T) {
	t.Parallel()
	ps := newTestSource(t)
	list, err := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	for _, tool := range list {
		if tool.Name() == "advanced_search" {
			t.Fatal("advanced_search should be hidden until revealed")
		}
	}
}

func TestProgressiveRevealCallableTool(t *testing.T) {
	t.Parallel()
	ps := newTestSource(t)
	list, err := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	reveal := find(t, list, "tool_reveal")
	if _, err := reveal.Call(context.Background(), json.RawMessage(`{"name":"advanced_search","reason":"need details"}`)); err != nil {
		t.Fatalf("tool_reveal: %v", err)
	}
	list, err = ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	if err != nil {
		t.Fatalf("Tools after reveal: %v", err)
	}
	out, err := find(t, list, "advanced_search").Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("advanced_search call: %v", err)
	}
	if out != "advanced-result" {
		t.Fatalf("out = %q", out)
	}
}

func TestProgressiveRevealSessionIsolation(t *testing.T) {
	t.Parallel()
	ps := newTestSource(t)
	list, _ := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	_, _ = find(t, list, "tool_reveal").Call(context.Background(), json.RawMessage(`{"name":"advanced_search","reason":"need details"}`))
	other, err := ps.Tools(context.Background(), coretools.TurnInfo{Session: "b"})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	for _, tool := range other {
		if tool.Name() == "advanced_search" {
			t.Fatal("advanced_search leaked across sessions")
		}
	}
}

func TestProgressiveRevealSafetyPolicy(t *testing.T) {
	t.Parallel()
	child := faketools.NewSource(faketools.New("danger", "done"))
	ps, err := progressive.NewSource(child, progressive.Manifest{
		Version:        1,
		DefaultVisible: []string{},
		Tools: []progressive.ManifestTool{{
			Name:   "danger",
			Group:  "danger",
			Safety: progressive.Safety{Destructive: true},
			Reveal: progressive.RevealPolicy{Mode: "explicit", ReasonRequired: true},
		}},
	})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	list, _ := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	_, err = find(t, list, "tool_reveal").Call(context.Background(), json.RawMessage(`{"name":"danger","reason":"need it"}`))
	if err == nil || !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("expected destructive policy error, got %v", err)
	}
}

func TestProgressiveManifestDependencies(t *testing.T) {
	t.Parallel()
	child := faketools.NewSource(faketools.New("a", "a"), faketools.New("b", "b"))
	_, err := progressive.NewSource(child, progressive.Manifest{
		Version: 1,
		Tools: []progressive.ManifestTool{
			{Name: "a", Reveal: progressive.RevealPolicy{Mode: "explicit", Requires: []string{"b"}}},
			{Name: "b", Reveal: progressive.RevealPolicy{Mode: "explicit", Requires: []string{"a"}}},
		},
	})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestProgressiveDiscoveryTools(t *testing.T) {
	t.Parallel()
	ps := newTestSource(t)
	list, _ := ps.Tools(context.Background(), coretools.TurnInfo{Session: "a"})
	out, err := find(t, list, "tool_list").Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("tool_list: %v", err)
	}
	if !strings.Contains(out, "advanced_search") || strings.Contains(out, "parameters") {
		t.Fatalf("tool_list should be bounded summary, got %s", out)
	}
	help, err := find(t, list, "tool_help").Call(context.Background(), json.RawMessage(`{"name":"advanced_search"}`))
	if err != nil {
		t.Fatalf("tool_help: %v", err)
	}
	if !strings.Contains(help, "compact") || !strings.Contains(help, "none") {
		t.Fatalf("tool_help missing output/data access summary: %s", help)
	}
}

func newTestSource(t *testing.T) *progressive.Source {
	t.Helper()
	child := faketools.NewSource(faketools.New("read_file", "read"), faketools.New("advanced_search", "advanced-result"))
	ps, err := progressive.NewSource(child, progressive.Manifest{
		Version:        1,
		DefaultVisible: []string{"read_file"},
		Tools: []progressive.ManifestTool{{
			Name:        "read_file",
			Group:       "files",
			Description: "Read a file.",
			Output:      progressive.OutputContract{Mode: "compact", MaxOutputBytes: 1024},
			DataAccess:  progressive.DataAccess{Mode: "none"},
		}, {
			Name:        "advanced_search",
			Group:       "search",
			Description: "Search deeply.",
			Reveal:      progressive.RevealPolicy{Mode: "explicit", ReasonRequired: true},
			Output:      progressive.OutputContract{Mode: "compact", MaxOutputBytes: 1024},
			DataAccess:  progressive.DataAccess{Mode: "none"},
		}},
		Groups: []progressive.Group{{Name: "files", MaxTools: 5}, {Name: "search", MaxTools: 5}},
	})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	return ps
}

func names(list []coretools.Tool) []string {
	out := make([]string, len(list))
	for i, tool := range list {
		out[i] = tool.Name()
	}
	return out
}

func find(t *testing.T, list []coretools.Tool, name string) coretools.Tool {
	t.Helper()
	for _, tool := range list {
		if tool.Name() == name {
			return tool
		}
	}
	t.Fatalf("tool %s not found in %v", name, names(list))
	return nil
}
