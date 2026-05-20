package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PatternManifest struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Budget  any    `json:"budget,omitempty"`
	Steps   []Step `json:"steps,omitempty"`
}

type Step struct {
	ID    string `json:"id"`
	Agent string `json:"agent,omitempty"`
	Tool  string `json:"tool,omitempty"`
}

func LoadPatternManifests(dir string) ([]PatternManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []PatternManifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m PatternManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("load pattern manifest %s: %w", e.Name(), err)
		}
		if err := ValidatePatternManifest(m); err != nil {
			return nil, fmt.Errorf("validate pattern manifest %s: %w", e.Name(), err)
		}
		out = append(out, m)
	}
	return out, nil
}

func ValidatePatternManifest(m PatternManifest) error {
	if m.Name == "" {
		return fmt.Errorf("name: required")
	}
	if m.Pattern == "" {
		return fmt.Errorf("pattern: required")
	}
	return nil
}
