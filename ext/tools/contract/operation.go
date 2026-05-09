// Package contract defines operation contracts and adapters for nanogo tools.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/tvmaly/nanogo/core/tools"
)

type Operation struct {
	Tool         string
	Name         string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Output       OutputContract
	ReadOnly     bool
	Safety       Safety
	DataAccess   DataAccess
	Examples     []Example
	Tags         []string
	Invoke       func(context.Context, json.RawMessage) (json.RawMessage, error)
}

type OutputContract struct {
	Mode           string
	MaxOutputBytes int
	MaxItems       int
}

type DataAccess struct {
	Mode      string
	Freshness string
	SyncTool  string
}

type Safety struct {
	RequiresApproval bool `json:"requires_approval,omitempty"`
	Destructive      bool `json:"destructive,omitempty"`
	Network          bool `json:"network,omitempty"`
	Filesystem       bool `json:"filesystem,omitempty"`
	ChildData        bool `json:"child_data,omitempty"`
}

type Example struct {
	Name   string          `json:"name,omitempty"`
	Args   json.RawMessage `json:"args,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
}

type ToolError struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
	Retry      bool     `json:"retry"`
	Available  []string `json:"available,omitempty"`
}

func (e ToolError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func NewError(code, message, suggestion string, retry bool) ToolError {
	return ToolError{Code: code, Message: message, Suggestion: suggestion, Retry: retry}
}

type adapter struct {
	op     Operation
	schema json.RawMessage
}

func Adapt(op Operation) (tools.Tool, error) {
	if err := Validate(op); err != nil {
		return nil, err
	}
	schema, err := json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        op.Name,
			"description": op.Description,
			"parameters":  json.RawMessage(op.InputSchema),
		},
	})
	if err != nil {
		return nil, err
	}
	return &adapter{op: op, schema: schema}, nil
}

func (a *adapter) Name() string { return a.op.Name }

func (a *adapter) Schema() json.RawMessage {
	out := make([]byte, len(a.schema))
	copy(out, a.schema)
	return out
}

func (a *adapter) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if err := validateArgs(a.op.InputSchema, args); err != nil {
		terr := NewError("usage", err.Error(), "fix arguments to match the input schema", false)
		return structuredError(terr), terr
	}
	out, err := a.op.Invoke(ctx, args)
	if err != nil {
		var terr ToolError
		if errors.As(err, &terr) {
			return structuredError(terr), err
		}
		terr = NewError("api", err.Error(), "", false)
		return structuredError(terr), err
	}
	if bytes.IndexByte(out, 0) >= 0 {
		terr := NewError("unsafe", "operation returned binary-looking output", "return JSON text output", false)
		return structuredError(terr), terr
	}
	out = compactJSON(out)
	if a.op.Output.MaxOutputBytes > 0 && len(out) > a.op.Output.MaxOutputBytes {
		summary, _ := json.Marshal(map[string]any{
			"truncated": true,
			"bytes":     len(out),
			"limit":     a.op.Output.MaxOutputBytes,
		})
		return string(summary), nil
	}
	return string(out), nil
}

func Validate(op Operation) error {
	if op.Name == "" {
		return fmt.Errorf("operation name is required")
	}
	if op.Description == "" {
		return fmt.Errorf("operation %s description is required", op.Name)
	}
	if len(op.InputSchema) == 0 {
		return fmt.Errorf("operation %s input schema is required", op.Name)
	}
	if len(op.OutputSchema) == 0 {
		return fmt.Errorf("operation %s output schema is required", op.Name)
	}
	if !validJSON(op.InputSchema) || !validJSON(op.OutputSchema) {
		return fmt.Errorf("operation %s schemas must be valid JSON", op.Name)
	}
	switch op.Output.Mode {
	case "compact", "full", "summary":
	default:
		return fmt.Errorf("operation %s invalid output mode %q", op.Name, op.Output.Mode)
	}
	if op.Output.MaxOutputBytes <= 0 {
		return fmt.Errorf("operation %s output must be bounded", op.Name)
	}
	switch op.DataAccess.Mode {
	case "none", "local", "live", "auto":
	default:
		return fmt.Errorf("operation %s invalid data access mode %q", op.Name, op.DataAccess.Mode)
	}
	if (op.DataAccess.Mode == "local" || op.DataAccess.Mode == "auto") && op.DataAccess.Freshness == "" {
		return fmt.Errorf("operation %s freshness metadata is required for %s data", op.Name, op.DataAccess.Mode)
	}
	if op.Invoke == nil {
		return fmt.Errorf("operation %s invoke is required", op.Name)
	}
	for _, ex := range op.Examples {
		if len(ex.Args) > 0 {
			if err := validateArgs(op.InputSchema, ex.Args); err != nil {
				return fmt.Errorf("operation %s example %q: %w", op.Name, ex.Name, err)
			}
		}
	}
	return nil
}

func HasTag(op Operation, tag string) bool {
	for _, t := range op.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func structuredError(err ToolError) string {
	b, _ := json.Marshal(map[string]any{"error": err})
	return string(b)
}

func compactJSON(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return append(json.RawMessage(nil), buf.Bytes()...)
}

func validJSON(raw json.RawMessage) bool {
	var v any
	return json.Unmarshal(raw, &v) == nil
}

func validateArgs(schema, args json.RawMessage) error {
	var spec struct {
		Type                 string                       `json:"type"`
		Properties           map[string]map[string]string `json:"properties"`
		Required             []string                     `json:"required"`
		AdditionalProperties *bool                        `json:"additionalProperties"`
	}
	if err := json.Unmarshal(schema, &spec); err != nil {
		return fmt.Errorf("invalid input schema: %w", err)
	}
	if spec.Type != "" && spec.Type != "object" {
		return fmt.Errorf("input schema type %q is not supported", spec.Type)
	}
	var got map[string]any
	if err := json.Unmarshal(args, &got); err != nil {
		return fmt.Errorf("invalid JSON args: %w", err)
	}
	for _, req := range spec.Required {
		if _, ok := got[req]; !ok {
			return fmt.Errorf("missing required field %q", req)
		}
	}
	for key, value := range got {
		prop, ok := spec.Properties[key]
		if !ok {
			if spec.AdditionalProperties != nil && !*spec.AdditionalProperties {
				return fmt.Errorf("unknown field %q", key)
			}
			continue
		}
		if want := prop["type"]; want != "" && !matchesType(value, want) {
			return fmt.Errorf("field %q has wrong type", key)
		}
	}
	return nil
}

func matchesType(v any, want string) bool {
	switch want {
	case "string":
		_, ok := v.(string)
		return ok
	case "integer":
		n, ok := v.(float64)
		return ok && n == float64(int64(n))
	case "number":
		_, ok := v.(float64)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	default:
		return true
	}
}

type Manifest struct {
	Tools []ManifestTool `json:"tools"`
}

type ManifestTool struct {
	Name       string         `json:"name"`
	Safety     Safety         `json:"safety,omitempty"`
	Output     OutputContract `json:"output,omitempty"`
	DataAccess DataAccess     `json:"data_access,omitempty"`
	Examples   []Example      `json:"examples,omitempty"`
}

func ValidateManifest(m Manifest, ops []Operation) error {
	byName := make(map[string]Operation, len(ops))
	for _, op := range ops {
		byName[op.Name] = op
	}
	seen := map[string]bool{}
	for _, mt := range m.Tools {
		if mt.Name == "" {
			return fmt.Errorf("manifest tool name is required")
		}
		if seen[mt.Name] {
			return fmt.Errorf("duplicate manifest tool %q", mt.Name)
		}
		seen[mt.Name] = true
		op, ok := byName[mt.Name]
		if !ok {
			return fmt.Errorf("manifest tool %q has no operation", mt.Name)
		}
		if !reflect.DeepEqual(mt.Safety, Safety{}) && !reflect.DeepEqual(mt.Safety, op.Safety) {
			return fmt.Errorf("manifest tool %q safety metadata mismatch", mt.Name)
		}
		if !reflect.DeepEqual(mt.Output, OutputContract{}) && !reflect.DeepEqual(mt.Output, op.Output) {
			return fmt.Errorf("manifest tool %q output metadata mismatch", mt.Name)
		}
		if !reflect.DeepEqual(mt.DataAccess, DataAccess{}) && !reflect.DeepEqual(mt.DataAccess, op.DataAccess) {
			return fmt.Errorf("manifest tool %q data access metadata mismatch", mt.Name)
		}
		for _, ex := range mt.Examples {
			if len(ex.Args) > 0 {
				if err := validateArgs(op.InputSchema, ex.Args); err != nil {
					return fmt.Errorf("manifest tool %q example %q: %w", mt.Name, ex.Name, err)
				}
			}
		}
	}
	return nil
}

type GeneratedTool struct {
	Operation                 Operation
	Manifest                  Manifest
	HasTests                  bool
	HasCompactOutputFixture   bool
	HasStructuredErrorFixture bool
}

func ValidateGeneratedTool(g GeneratedTool) error {
	if err := Validate(g.Operation); err != nil {
		return err
	}
	if err := ValidateManifest(g.Manifest, []Operation{g.Operation}); err != nil {
		return err
	}
	if !g.HasTests {
		return fmt.Errorf("generated tool %s missing tests", g.Operation.Name)
	}
	if !g.HasCompactOutputFixture {
		return fmt.Errorf("generated tool %s missing compact output fixture", g.Operation.Name)
	}
	if !g.HasStructuredErrorFixture {
		return fmt.Errorf("generated tool %s missing structured error fixture", g.Operation.Name)
	}
	return nil
}

type FixtureSet struct {
	CompactOutput   json.RawMessage
	StructuredError json.RawMessage
}

func CompareFixtures(want, got FixtureSet) error {
	if !jsonEqual(want.CompactOutput, got.CompactOutput) {
		return fmt.Errorf("compact output fixture drift")
	}
	if !jsonEqual(want.StructuredError, got.StructuredError) {
		return fmt.Errorf("structured error fixture drift")
	}
	return nil
}

func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
	}
	return reflect.DeepEqual(normalize(av), normalize(bv))
}

func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys)*2)
		for _, k := range keys {
			out = append(out, k, normalize(x[k]))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = normalize(x[i])
		}
		return out
	default:
		return x
	}
}
