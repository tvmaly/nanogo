package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvmaly/nanogo/ext/agentpatterns/manifest"
)

func TestLoadPatternManifestsValidatesWorkspaceAssets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fraction.json"), []byte(`{"name":"fraction_lesson","pattern":"sequential","steps":[{"id":"assess","agent":"math"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := manifest.LoadPatternManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "fraction_lesson" || got[0].Pattern != "sequential" {
		t.Fatalf("manifests = %#v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"name":"","pattern":"missing"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = manifest.LoadPatternManifests(dir)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want validation", err)
	}
}
