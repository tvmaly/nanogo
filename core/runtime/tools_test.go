package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	coreruntime "github.com/tvmaly/nanogo/core/runtime"
	"github.com/tvmaly/nanogo/core/tools"
)

func TestToolSourceDuplicateNames(t *testing.T) {
	t.Parallel()
	src := coreruntime.NewMultiSource(newNamedSource("a", &namedTool{name: "same"}), newNamedSource("b", &namedTool{name: "same"}))
	_, err := src.Tools(context.Background(), tools.TurnInfo{})
	if err == nil {
		t.Fatal("expected duplicate tool name error")
	}
	if !strings.Contains(err.Error(), "same") {
		t.Fatalf("error should name duplicate tool, got %v", err)
	}
}

func TestMultiSourceDeterministicOrder(t *testing.T) {
	t.Parallel()
	src := coreruntime.NewMultiSource(newNamedSource("b", &namedTool{name: "z"}), newNamedSource("a", &namedTool{name: "a"}))
	list, err := src.Tools(context.Background(), tools.TurnInfo{})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := list[0].Name() + "," + list[1].Name(); got != "a,z" {
		t.Fatalf("order = %s", got)
	}
}

type namedSource struct {
	name string
	list []tools.Tool
}

func newNamedSource(name string, list ...tools.Tool) *namedSource {
	return &namedSource{name: name, list: list}
}

func (s *namedSource) SourceName() string { return s.name }

func (s *namedSource) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) {
	return s.list, nil
}

type namedTool struct{ name string }

func (t *namedTool) Name() string { return t.name }
func (t *namedTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"function","function":{"name":"` + t.name + `","description":"test","parameters":{"type":"object","properties":{}}}}`)
}
func (t *namedTool) Call(context.Context, json.RawMessage) (string, error) {
	return t.name, nil
}
