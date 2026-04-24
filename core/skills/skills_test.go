package skills_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/core/skills"
	fakeskills "github.com/tvmaly/nanogo/core/skills/fake"
)

func TestFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("full frontmatter", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testdataDir(t), "deploy-service.md")
		sk, err := skills.ParseFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sk.Name != "deploy-service" {
			t.Errorf("Name = %q, want deploy-service", sk.Name)
		}
		if sk.Description == "" {
			t.Error("Description is empty")
		}
		if sk.Kind != "skill" {
			t.Errorf("Kind = %q, want skill", sk.Kind)
		}
		if sk.Path != path {
			t.Errorf("Path = %q, want %q", sk.Path, path)
		}
		if sk.Body == "" {
			t.Error("Body is empty")
		}
		if sk.Frontmatter == nil {
			t.Error("Frontmatter is nil")
		}
	})

	t.Run("no frontmatter treated as body", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "plain.md")
		os.WriteFile(p, []byte("just a body"), 0644)
		sk, err := skills.ParseFile(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sk.Body != "just a body" {
			t.Errorf("Body = %q, want 'just a body'", sk.Body)
		}
		if sk.Name != "plain" {
			t.Errorf("Name = %q, want 'plain'", sk.Name)
		}
	})

	t.Run("malformed YAML returns error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "bad.md")
		os.WriteFile(p, []byte("---\nname: [unclosed\n---\nbody"), 0644)
		_, err := skills.ParseFile(p)
		if err == nil {
			t.Fatal("expected error for malformed YAML")
		}
	})
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	t.Run("finds files with frontmatter, ignores plain md", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeSkill(t, dir, "a.md", "---\nname: alpha\ndescription: A\n---\nbody")
		writeSkill(t, dir, "b.md", "---\nname: beta\ndescription: B\n---\nbody")
		os.WriteFile(filepath.Join(dir, "plain.md"), []byte("no frontmatter"), 0644)

		got, err := skills.Discover(dir, nil)
		if err != nil {
			t.Fatalf("Discover error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d skills, want 2", len(got))
		}
	})

	t.Run("duplicate names return error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeSkill(t, dir, "a.md", "---\nname: same\n---\nbody1")
		writeSkill(t, dir, "b.md", "---\nname: same\n---\nbody2")
		_, err := skills.Discover(dir, nil)
		if err == nil {
			t.Fatal("expected error for duplicate names")
		}
		if !strings.Contains(err.Error(), "same") {
			t.Errorf("error %q should mention duplicate name", err.Error())
		}
	})
}

func TestDispatchHappy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkill(t, dir, "deploy-service.md", "---\nname: deploy-service\nargs:\n  - env\n  - service\n---\nDeploy {{service}} to {{env}}.")

	sk, _ := skills.Discover(dir, nil)
	loader := skills.NewLoader(sk)
	runner := &fakeskills.AgentRunner{}
	d := skills.NewDispatcher(loader, runner)

	err := d.Fire(context.Background(), skills.Trigger{
		Skill:  "deploy-service",
		Source: skills.SourceCLI,
		Args:   map[string]any{"env": "dev", "service": "api"},
	})
	if err != nil {
		t.Fatalf("Fire error: %v", err)
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("expected 1 RunSkill call, got %d", len(runner.Calls))
	}
	call := runner.Calls[0]
	if !strings.Contains(call.UserMsg, "api") || !strings.Contains(call.UserMsg, "dev") {
		t.Errorf("UserMsg %q missing substituted values", call.UserMsg)
	}
	if call.SystemNote != "" {
		t.Errorf("expected no SystemNote, got %q", call.SystemNote)
	}
}

func TestDispatchMissingArgs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkill(t, dir, "deploy-service.md", "---\nname: deploy-service\nargs:\n  - env\n  - service\n---\nDeploy {{service}} to {{env}}.")

	sk, _ := skills.Discover(dir, nil)
	loader := skills.NewLoader(sk)
	runner := &fakeskills.AgentRunner{}
	d := skills.NewDispatcher(loader, runner)

	err := d.Fire(context.Background(), skills.Trigger{
		Skill:  "deploy-service",
		Source: skills.SourceCLI,
		Args:   map[string]any{"service": "api"}, // env is missing
	})
	if err != nil {
		t.Fatalf("Fire error: %v", err)
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("expected 1 RunSkill call, got %d", len(runner.Calls))
	}
	call := runner.Calls[0]
	if !strings.Contains(call.SystemNote, "env") {
		t.Errorf("SystemNote %q should mention missing arg 'env'", call.SystemNote)
	}
	if !strings.Contains(call.SystemNote, "ask_user") {
		t.Errorf("SystemNote %q should instruct use of ask_user", call.SystemNote)
	}
}

func TestTemplate(t *testing.T) {
	t.Parallel()

	t.Run("substitutes known key", func(t *testing.T) {
		t.Parallel()
		out, err := skills.Substitute("Hello {{name}}!", map[string]any{"name": "world"})
		if err != nil {
			t.Fatal(err)
		}
		if out != "Hello world!" {
			t.Errorf("got %q, want 'Hello world!'", out)
		}
	})

	t.Run("missing key resolves to empty string", func(t *testing.T) {
		t.Parallel()
		out, err := skills.Substitute("Hello {{missing}}!", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if out != "Hello !" {
			t.Errorf("got %q, want 'Hello !'", out)
		}
	})

	t.Run("no panic on undefined keys", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panicked: %v", r)
			}
		}()
		skills.Substitute("{{a}} {{b}} {{c}}", map[string]any{"a": "x"})
	})
}

func TestSkillModel(t *testing.T) {
	t.Parallel()

	t.Run("valid route loads without error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeSkill(t, dir, "cheap-model.md", "---\nname: cheap-model-skill\nmodel: cheap\n---\nbody")
		_, err := skills.Discover(dir, []string{"cheap", "standard"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown route returns error with available routes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeSkill(t, dir, "bad.md", "---\nname: bad-route\nmodel: unknown-route\n---\nbody")
		_, err := skills.Discover(dir, []string{"cheap", "standard"})
		if err == nil {
			t.Fatal("expected error for unknown route")
		}
		if !strings.Contains(err.Error(), "unknown-route") {
			t.Errorf("error %q should mention the bad route", err.Error())
		}
		if !strings.Contains(err.Error(), "cheap") {
			t.Errorf("error %q should list available routes", err.Error())
		}
	})

	t.Run("model field set on loaded skill", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testdataDir(t), "cheap-model.md")
		sk, err := skills.ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if sk.Model != "cheap" {
			t.Errorf("Model = %q, want 'cheap'", sk.Model)
		}
	})

	t.Run("dispatcher passes skill name in opts", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeSkill(t, dir, "s.md", "---\nname: myskill\nmodel: cheap\n---\nbody")
		skList, _ := skills.Discover(dir, nil)
		loader := skills.NewLoader(skList)
		runner := &fakeskills.AgentRunner{}
		d := skills.NewDispatcher(loader, runner)
		d.Fire(context.Background(), skills.Trigger{Skill: "myskill", Source: skills.SourceCLI})
		if len(runner.Calls) == 0 {
			t.Fatal("no calls")
		}
		if runner.Calls[0].SkillName != "myskill" {
			t.Errorf("SkillName = %q, want 'myskill'", runner.Calls[0].SkillName)
		}
		if runner.Calls[0].Model != "cheap" {
			t.Errorf("Model = %q, want 'cheap'", runner.Calls[0].Model)
		}
	})
}

func TestSkillTools(t *testing.T) {
	t.Parallel()

	t.Run("subagent skill carries tools allowlist", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testdataDir(t), "code-reviewer.md")
		sk, err := skills.ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if sk.Kind != "subagent" {
			t.Errorf("Kind = %q, want subagent", sk.Kind)
		}
		wantTools := map[string]bool{"read_file": true, "bash": true}
		for _, tool := range sk.Tools {
			if !wantTools[tool] {
				t.Errorf("unexpected tool %q", tool)
			}
			delete(wantTools, tool)
		}
		for tool := range wantTools {
			t.Errorf("missing expected tool %q", tool)
		}
	})

	t.Run("regular skill tools field is nil", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(testdataDir(t), "deploy-service.md")
		sk, err := skills.ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(sk.Tools) != 0 {
			t.Errorf("expected no tools on regular skill, got %v", sk.Tools)
		}
	})
}

func TestSpawnSubagentSkill(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir(t), "code-reviewer.md")
	sk, err := skills.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	loader := skills.NewLoader([]*skills.Skill{sk})
	runner := &fakeskills.AgentRunner{Result: "LGTM"}
	d := skills.NewDispatcher(loader, runner)

	err = d.Fire(context.Background(), skills.Trigger{
		Skill:  "code-reviewer",
		Source: skills.SourceLLM,
		Args:   map[string]any{},
	})
	if err != nil {
		t.Fatalf("Fire error: %v", err)
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.Calls))
	}
	call := runner.Calls[0]
	if call.Model != "cheap" {
		t.Errorf("Model = %q, want 'cheap'", call.Model)
	}
	if len(call.Tools) == 0 {
		t.Error("expected tools to be set for subagent skill")
	}
	if !strings.Contains(call.UserMsg, "code reviewer") {
		t.Errorf("UserMsg should contain skill body, got %q", call.UserMsg)
	}
}

func TestSkillKindSubagentFilteredFromList(t *testing.T) {
	t.Parallel()

	sks, err := skills.Discover(testdataDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := skills.NewLoader(sks)

	for _, sk := range loader.UserFacing() {
		if sk.Kind == "subagent" {
			t.Errorf("UserFacing returned subagent skill %q", sk.Name)
		}
	}

	found := false
	for _, sk := range loader.All() {
		if sk.Kind == "subagent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("All() should include subagent skills")
	}
}

// helpers

func testdataDir(t *testing.T) string {
	t.Helper()
	// Walk up from the package dir to find testdata/skills
	dir, err := filepath.Abs("../../testdata/skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata/skills not found at %s: %v", dir, err)
	}
	return dir
}

func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
