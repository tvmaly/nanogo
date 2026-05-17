// Package workspace validates editable workspace capability assets.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ToolManifest is the required metadata in workspace/tools/<name>/tool.yaml.
type ToolManifest struct {
	Name        string
	Command     string
	Description string
}

// ToolCapability is a validated workspace tool triplet.
type ToolCapability struct {
	Dir      string
	Manifest ToolManifest
	Prompt   string
	Tests    string
}

// LoadTools loads workspace/tools/* capability triplets in deterministic order.
func LoadTools(root string) ([]ToolCapability, error) {
	base := filepath.Join(root, "tools")
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("workspace tools: %w", err)
	}
	seen := map[string]string{}
	var out []ToolCapability
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		capability, err := LoadTool(filepath.Join(base, entry.Name()))
		if err != nil {
			return nil, err
		}
		name := capability.Manifest.Name
		if prev, ok := seen[name]; ok {
			return nil, fmt.Errorf("tool.yaml name: duplicate %q in %s and %s", name, prev, capability.Dir)
		}
		seen[name] = capability.Dir
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.Name < out[j].Manifest.Name
	})
	return out, nil
}

// LoadTool loads one workspace/tools/<name> directory.
func LoadTool(dir string) (ToolCapability, error) {
	clean := filepath.Clean(dir)
	manifest, err := parseManifest(filepath.Join(clean, "tool.yaml"))
	if err != nil {
		return ToolCapability{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return ToolCapability{}, err
	}
	prompt, err := readRequired(filepath.Join(clean, "prompt.md"), "prompt.md")
	if err != nil {
		return ToolCapability{}, err
	}
	tests, err := readRequired(filepath.Join(clean, "tests.yaml"), "tests.yaml")
	if err != nil {
		return ToolCapability{}, err
	}
	return ToolCapability{Dir: clean, Manifest: manifest, Prompt: prompt, Tests: tests}, nil
}

func parseManifest(path string) (ToolManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolManifest{}, fmt.Errorf("tool.yaml: %w", err)
	}
	var m ToolManifest
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return ToolManifest{}, fmt.Errorf("tool.yaml: malformed line %q", line)
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			m.Name = value
		case "command":
			m.Command = value
		case "description":
			m.Description = value
		default:
			return ToolManifest{}, fmt.Errorf("tool.yaml.%s: unknown field", strings.TrimSpace(key))
		}
	}
	return m, nil
}

func validateManifest(m ToolManifest) error {
	if m.Name == "" {
		return errors.New("tool.yaml.name: required")
	}
	if !validName.MatchString(m.Name) {
		return fmt.Errorf("tool.yaml.name: invalid %q", m.Name)
	}
	if strings.Contains(m.Name, "..") || strings.ContainsAny(m.Name, `/\`) {
		return fmt.Errorf("tool.yaml.name: path traversal not allowed: %q", m.Name)
	}
	if m.Command == "" {
		return errors.New("tool.yaml.command: required")
	}
	if strings.Contains(m.Command, "..") {
		return fmt.Errorf("tool.yaml.command: path traversal not allowed: %q", m.Command)
	}
	return nil
}

func readRequired(path, field string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("%s: required", field)
	}
	return string(data), nil
}
