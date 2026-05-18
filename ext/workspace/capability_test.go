package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/workspace"
)

func TestLoadToolsValidatesTripletsDeterministically(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTool(t, root, "zeta", "zeta", "echo z")
	writeTool(t, root, "alpha", "alpha", "echo a")

	tools, err := workspace.LoadTools(root)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len = %d, want 2", len(tools))
	}
	if tools[0].Manifest.Name != "alpha" || tools[1].Manifest.Name != "zeta" {
		t.Fatalf("tools not sorted by name: %#v", tools)
	}
	if !strings.Contains(tools[0].Prompt, "prompt") || !strings.Contains(tools[0].Tests, "cases") {
		t.Fatalf("prompt/tests not loaded: %#v", tools[0])
	}
	if tools[0].Manifest.SchemaVersion != "workspace.tool.v1" {
		t.Fatalf("schema version = %q, want workspace.tool.v1", tools[0].Manifest.SchemaVersion)
	}
}

func TestLoadToolsAcceptsExplicitSchemaVersion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTool(t, root, "alpha", "alpha", "echo a")
	writeFile(t, filepath.Join(root, "tools", "alpha", "tool.yaml"), "schema_version: workspace.tool.v1\nname: alpha\ncommand: echo a\ndescription: test\n")
	tools, err := workspace.LoadTools(root)
	if err != nil {
		t.Fatalf("LoadTools: %v", err)
	}
	if tools[0].Manifest.SchemaVersion != "workspace.tool.v1" {
		t.Fatalf("schema version = %q", tools[0].Manifest.SchemaVersion)
	}
}

func TestLoadToolsRejectsUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTool(t, root, "alpha", "alpha", "echo a")
	writeFile(t, filepath.Join(root, "tools", "alpha", "tool.yaml"), "schema_version: workspace.tool.v2\nname: alpha\ncommand: echo a\ndescription: test\n")
	_, err := workspace.LoadTools(root)
	if err == nil || !strings.Contains(err.Error(), "tool.yaml.schema_version") {
		t.Fatalf("err = %v, want schema_version error", err)
	}
}

func TestLoadToolsInvalidManifestFailsClearly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(root string)
		want string
	}{
		{
			name: "missing command",
			edit: func(root string) {
				writeTool(t, root, "bad", "bad", "")
				writeFile(t, filepath.Join(root, "tools", "bad", "tool.yaml"), "name: bad\n")
			},
			want: "tool.yaml.command",
		},
		{
			name: "bad schema",
			edit: func(root string) {
				writeTool(t, root, "bad", "bad", "echo ok")
				writeFile(t, filepath.Join(root, "tools", "bad", "tool.yaml"), "name: bad\ncommand: echo ok\nextra: nope\n")
			},
			want: "tool.yaml.extra",
		},
		{
			name: "path traversal",
			edit: func(root string) {
				writeTool(t, root, "bad", "bad", "../run")
			},
			want: "tool.yaml.command",
		},
		{
			name: "duplicate",
			edit: func(root string) {
				writeTool(t, root, "one", "dup", "echo one")
				writeTool(t, root, "two", "dup", "echo two")
			},
			want: "duplicate",
		},
		{
			name: "missing tests",
			edit: func(root string) {
				writeTool(t, root, "bad", "bad", "echo ok")
				if err := os.Remove(filepath.Join(root, "tools", "bad", "tests.yaml")); err != nil {
					t.Fatal(err)
				}
			},
			want: "tests.yaml",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tt.edit(root)
			_, err := workspace.LoadTools(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func writeTool(t *testing.T, root, dir, name, command string) {
	t.Helper()
	base := filepath.Join(root, "tools", dir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(base, "tool.yaml"), "name: "+name+"\ncommand: "+command+"\ndescription: test\n")
	writeFile(t, filepath.Join(base, "prompt.md"), "prompt\n")
	writeFile(t, filepath.Join(base, "tests.yaml"), "cases:\n  - name: ok\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
