package skills_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	"github.com/tvmaly/nanogo/modules/skills"
	fakeskills "github.com/tvmaly/nanogo/modules/skills/fake"
)

func TestSkillPatternOptInUsesPatternRunnerContract(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "fraction.md", "---\nname: fraction-helper\nmode: agentpatterns\npattern: sequential\npattern_manifest: fraction_lesson\nargs:\n  - problem\n---\nHelp solve {{problem}}.")
	list, _ := skills.Discover(dir, nil)
	runner := &fakeskills.AgentRunner{}
	patterns := &contractfake.PatternRuntime{RunResult: contracts.PatternResult{Text: "pattern done"}}
	d := skills.NewDispatcherWithPatterns(skills.NewLoader(list), runner, patterns)

	err := d.Fire(context.Background(), skills.Trigger{
		Skill: "fraction-helper", Source: skills.SourceCLI, Session: "s1",
		Args: map[string]any{"problem": "1/2+1/4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 0 || len(patterns.RunRequests) != 1 {
		t.Fatalf("agent calls=%d pattern calls=%d", len(runner.Calls), len(patterns.RunRequests))
	}
	req := patterns.RunRequests[0]
	if req.PatternHint != "sequential" || req.Metadata["pattern_manifest"] != "fraction_lesson" || req.SessionID != "s1" {
		t.Fatalf("request = %#v", req)
	}
}

func TestSkillWithoutPatternUsesExistingAgentPath(t *testing.T) {
	path := filepath.Join(testdataDir(t), "deploy-service.md")
	sk, err := skills.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeskills.AgentRunner{}
	patterns := &contractfake.PatternRuntime{}
	d := skills.NewDispatcherWithPatterns(skills.NewLoader([]*skills.Skill{sk}), runner, patterns)
	if err := d.Fire(context.Background(), skills.Trigger{Skill: sk.Name, Args: map[string]any{"env": "dev", "service": "api"}}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 1 || len(patterns.RunRequests) != 0 {
		t.Fatalf("agent calls=%d pattern calls=%d", len(runner.Calls), len(patterns.RunRequests))
	}
}
