package research_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coretools "github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/tools/research"
)

func TestFakeDriverWritesValidSourcesMarkdown(t *testing.T) {
	root := t.TempDir()
	src := research.NewSource(research.Config{Workspace: root, Enabled: true, Driver: "fake", Clock: fixedClock})
	out, err := src.Research(context.Background(), research.Request{Topic: "beginner yo-yo tricks", ChildAge: 7, SkillType: "physical"})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "inbox", "lessons", "sources", "beginner-yo-yo-tricks.sources.md")
	if out.SourcesPath != wantPath {
		t.Fatalf("path = %q want %q", out.SourcesPath, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{"schema_version: research.sources.v1", "skill_type: physical", "## Videos", "video_id:", "source: model_inferred", "selected_because:", "age_fit_note:", "skill_progression_note:", "parent_check_required:"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("sources.md missing %q\n%s", want, doc)
		}
	}
	if len(out.NeedsParentVerification) == 0 {
		t.Fatalf("expected inferred segment in parent verification summary: %+v", out)
	}
}

func TestNoVideoContentIsDownloaded(t *testing.T) {
	root := t.TempDir()
	src := research.NewSource(research.Config{Workspace: root, Enabled: true, Clock: fixedClock})
	if _, err := src.Research(context.Background(), research.Request{Topic: "beginner yo-yo tricks", ChildAge: 7}); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".mp4", ".webm", ".mkv", ".mov":
			t.Fatalf("video content was written: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestResearchConfigControlsWebSearchDomainsAndDisabledExposure(t *testing.T) {
	src := research.NewSource(research.Config{Enabled: true, WebSearch: research.WebSearchConfig{MaxResults: 8, ExcludedDomains: []string{"reddit.com"}}})
	mutation := src.ProviderWebSearchMutation()
	data, _ := json.Marshal(mutation)
	if !strings.Contains(string(data), "openrouter:web_search") || !strings.Contains(string(data), "reddit.com") {
		t.Fatalf("mutation = %s", data)
	}
	disabled := research.NewSource(research.Config{Enabled: false})
	tools, err := disabled.Tools(context.Background(), coretools.TurnInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("disabled source exposed tools: %+v", tools)
	}
}

func fixedClock() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
