package contract_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/tools/contract"
)

func TestOperationAdapter(t *testing.T) {
	t.Parallel()
	op := testOperation("echo")
	tool, err := contract.Adapt(op)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	if tool.Name() != "echo" {
		t.Fatalf("Name = %q", tool.Name())
	}
	var schema struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if schema.Type != "function" || schema.Function.Name != "echo" || len(schema.Function.Parameters) == 0 {
		t.Fatalf("bad schema: %s", tool.Schema())
	}
	out, err := tool.Call(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, `"echo":"hello"`) {
		t.Fatalf("out = %s", out)
	}
}

func TestOperationInputValidation(t *testing.T) {
	t.Parallel()
	called := false
	op := testOperation("echo")
	op.Invoke = func(context.Context, json.RawMessage) (json.RawMessage, error) {
		called = true
		return json.RawMessage(`{}`), nil
	}
	tool, err := contract.Adapt(op)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"text":3}`)); err == nil {
		t.Fatal("expected validation error")
	}
	if called {
		t.Fatal("Invoke ran after invalid input")
	}
}

func TestStructuredToolErrors(t *testing.T) {
	t.Parallel()
	op := testOperation("echo")
	op.Invoke = func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return nil, contract.NewError("rate_limit", "slow down", "retry later", true)
	}
	tool, err := contract.Adapt(op)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	out, err := tool.Call(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err == nil {
		t.Fatal("expected go error")
	}
	if !strings.Contains(out, `"code":"rate_limit"`) || !strings.Contains(out, `"retry":true`) {
		t.Fatalf("structured error output = %s", out)
	}
}

func TestOperationOutputContract(t *testing.T) {
	t.Parallel()
	op := testOperation("echo")
	op.Output.MaxOutputBytes = 40
	op.Invoke = func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"items":["abcdefghijklmnopqrstuvwxyz","abcdefghijklmnopqrstuvwxyz"]}`), nil
	}
	tool, err := contract.Adapt(op)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	out, err := tool.Call(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, `"truncated":true`) {
		t.Fatalf("expected truncation metadata, got %s", out)
	}
}

func TestOperationAgentNativeMetadata(t *testing.T) {
	t.Parallel()
	op := testOperation("echo")
	if err := contract.Validate(op); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	op.Output.Mode = "verbose"
	if err := contract.Validate(op); err == nil {
		t.Fatal("expected invalid output mode")
	}
	op = testOperation("echo")
	op.DataAccess.Mode = "local"
	op.DataAccess.Freshness = ""
	if err := contract.Validate(op); err == nil {
		t.Fatal("expected missing freshness error")
	}
}

func TestCompoundInsightOperationContract(t *testing.T) {
	t.Parallel()
	op := testOperation("project_health")
	op.Tags = []string{"insight", "compound"}
	op.Examples = []contract.Example{{Name: "health", Args: json.RawMessage(`{"text":"repo"}`), Output: json.RawMessage(`{"echo":"repo"}`)}}
	if err := contract.Validate(op); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !contract.HasTag(op, "compound") || !contract.HasTag(op, "insight") {
		t.Fatal("compound insight operation should be discoverable by tag")
	}
}

func TestOperationManifestValidation(t *testing.T) {
	t.Parallel()
	op := testOperation("echo")
	m := contract.Manifest{Tools: []contract.ManifestTool{{
		Name:       "echo",
		Safety:     op.Safety,
		Output:     op.Output,
		DataAccess: op.DataAccess,
		Examples:   []contract.Example{{Name: "ok", Args: json.RawMessage(`{"text":"hi"}`)}},
	}}}
	if err := contract.ValidateManifest(m, []contract.Operation{op}); err != nil {
		t.Fatalf("ValidateManifest: %v", err)
	}
	m.Tools[0].Output.Mode = "summary"
	if err := contract.ValidateManifest(m, []contract.Operation{op}); err == nil {
		t.Fatal("expected output metadata mismatch")
	}
}

func TestGeneratedToolScaffold(t *testing.T) {
	t.Parallel()
	g := contract.GeneratedTool{
		Operation:                 testOperation("echo"),
		Manifest:                  contract.Manifest{Tools: []contract.ManifestTool{{Name: "echo"}}},
		HasTests:                  true,
		HasCompactOutputFixture:   true,
		HasStructuredErrorFixture: true,
	}
	if err := contract.ValidateGeneratedTool(g); err != nil {
		t.Fatalf("ValidateGeneratedTool: %v", err)
	}
	g.HasCompactOutputFixture = false
	if err := contract.ValidateGeneratedTool(g); err == nil {
		t.Fatal("expected missing fixture error")
	}
}

func TestGeneratedToolOutputErrorFixtures(t *testing.T) {
	t.Parallel()
	golden := contract.FixtureSet{
		CompactOutput:   json.RawMessage(`{"echo":"hi"}`),
		StructuredError: json.RawMessage(`{"error":{"code":"usage","message":"bad","retry":false}}`),
	}
	got := contract.FixtureSet{
		CompactOutput:   json.RawMessage(`{"echo":"hi"}`),
		StructuredError: json.RawMessage(`{"error":{"code":"usage","message":"bad","retry":false}}`),
	}
	if err := contract.CompareFixtures(golden, got); err != nil {
		t.Fatalf("CompareFixtures: %v", err)
	}
	got.CompactOutput = json.RawMessage(`{"echo":"bye"}`)
	if err := contract.CompareFixtures(golden, got); err == nil {
		t.Fatal("expected drift error")
	}
}

func testOperation(name string) contract.Operation {
	return contract.Operation{
		Tool:         "test",
		Name:         name,
		Description:  "echo text",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"echo":{"type":"string"}}}`),
		Output:       contract.OutputContract{Mode: "compact", MaxOutputBytes: 256, MaxItems: 10},
		ReadOnly:     true,
		Safety:       contract.Safety{},
		DataAccess:   contract.DataAccess{Mode: "none"},
		Examples:     []contract.Example{{Name: "ok", Args: json.RawMessage(`{"text":"hi"}`), Output: json.RawMessage(`{"echo":"hi"}`)}},
		Invoke: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var in struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(args, &in)
			b, _ := json.Marshal(map[string]string{"echo": in.Text})
			return b, nil
		},
	}
}
