package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive/tools"
)

func TestAdaptiveTools(t *testing.T) {
	t.Parallel()
	src, err := tools.NewSource(tools.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	list, err := src.Tools(context.Background(), coretools.TurnInfo{})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	want := map[string]bool{
		"adaptive_profile_read": false, "adaptive_profile_update": false,
		"adaptive_archive_add": false, "adaptive_archive_top": false,
		"adaptive_outcome_record": false, "adaptive_experiment_create": false,
		"adaptive_experiment_run": false, "adaptive_inspect": false, "adaptive_report": false,
	}
	for _, tool := range list {
		if _, ok := want[tool.Name()]; ok {
			want[tool.Name()] = true
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Schema(), &schema); err != nil || schema["type"] != "function" {
			t.Fatalf("bad schema for %s: %v %#v", tool.Name(), err, schema)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing tool %s", name)
		}
	}
}

func TestProfileUpdateRequiresApproval(t *testing.T) {
	t.Parallel()
	src, _ := tools.NewSource(tools.Config{Root: t.TempDir(), RequireParentApprovalFor: []string{"new_child_profile_inference"}})
	list, _ := src.Tools(context.Background(), coretools.TurnInfo{})
	update := find(list, "adaptive_profile_update")
	out, err := update.Call(context.Background(), []byte(`{"child_id":"cross","field":"learning_style","value":"visual","reason":"new_child_profile_inference"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("invalid JSON output: %s", out)
	}
	read := find(list, "adaptive_profile_read")
	out, _ = read.Call(context.Background(), []byte(`{"child_id":"cross"}`))
	if string(out) == "" || contains(out, "visual") {
		t.Fatalf("pending update should not change active profile: %s", out)
	}
}

func find(list []coretools.Tool, name string) coretools.Tool {
	for _, t := range list {
		if t.Name() == name {
			return t
		}
	}
	panic(name)
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || contains(s[1:], sub) || s[:len(sub)] == sub))
}
