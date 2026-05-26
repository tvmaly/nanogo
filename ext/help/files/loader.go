// Package files loads help packs from the local filesystem.
package files

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tvmaly/nanogo/modules/help"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	Root string
}

func New(root string) Loader { return Loader{Root: root} }

type indexFile struct {
	SchemaVersion string   `yaml:"schema_version"`
	PackID        string   `yaml:"pack_id"`
	Name          string   `yaml:"name"`
	Version       int      `yaml:"version"`
	RootTopics    []string `yaml:"root_topics"`
}

type topicFrontmatter struct {
	SchemaVersion string   `yaml:"schema_version"`
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Summary       string   `yaml:"summary"`
	Kind          string   `yaml:"kind"`
	Interfaces    []string `yaml:"interfaces"`
	Audiences     []string `yaml:"audiences"`
	Tags          []string `yaml:"tags"`
	Related       []string `yaml:"related"`
	SourcePaths   []string `yaml:"source_paths"`
	Invariants    []string `yaml:"invariants"`
	LastVerified  string   `yaml:"last_verified"`
}

func (l Loader) Load(ctx context.Context) (help.Pack, error) {
	root, err := filepath.Abs(l.Root)
	if err != nil {
		return help.Pack{}, err
	}
	idxBytes, err := os.ReadFile(filepath.Join(root, "index.yaml"))
	if err != nil {
		return help.Pack{}, fmt.Errorf("read help index: %w", err)
	}
	var idx indexFile
	if err := yaml.Unmarshal(idxBytes, &idx); err != nil {
		return help.Pack{}, fmt.Errorf("decode help index: %w", err)
	}
	pack := help.Pack{SchemaVersion: idx.SchemaVersion, PackID: idx.PackID, Name: idx.Name, Version: idx.Version, RootTopics: idx.RootTopics}
	topicDir := filepath.Join(root, "topics")
	entries, err := os.ReadDir(topicDir)
	if err != nil {
		return help.Pack{}, fmt.Errorf("read help topics: %w", err)
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return help.Pack{}, ctx.Err()
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(topicDir, entry.Name())
		if !within(root, path) {
			return help.Pack{}, fmt.Errorf("unsafe help topic path %q", entry.Name())
		}
		topic, err := readTopic(path)
		if err != nil {
			return help.Pack{}, err
		}
		pack.Topics = append(pack.Topics, topic)
	}
	if errs := help.ValidatePack(pack); len(errs) > 0 {
		return help.Pack{}, help.ValidationError{Errors: errs}
	}
	return pack, nil
}

func readTopic(path string) (help.Topic, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return help.Topic{}, fmt.Errorf("read help topic %s: %w", path, err)
	}
	front, body, err := splitFrontmatter(b)
	if err != nil {
		return help.Topic{}, fmt.Errorf("parse help topic %s: %w", path, err)
	}
	var fm topicFrontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return help.Topic{}, fmt.Errorf("decode help topic %s: %w", path, err)
	}
	return help.Topic{
		SchemaVersion: fm.SchemaVersion,
		TopicMeta: help.TopicMeta{
			ID: fm.ID, Title: fm.Title, Summary: fm.Summary, Kind: fm.Kind,
			Interfaces: fm.Interfaces, Audiences: fm.Audiences, Tags: fm.Tags, Related: fm.Related,
		},
		SourcePaths: fm.SourcePaths, Invariants: fm.Invariants, LastVerified: fm.LastVerified,
		Sections: parseSections(body), Body: string(body),
	}, nil
}

func splitFrontmatter(b []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(b, []byte("---\n")) {
		return nil, nil, fmt.Errorf("missing frontmatter")
	}
	rest := b[len("---\n"):]
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx < 0 {
		return nil, nil, fmt.Errorf("unterminated frontmatter")
	}
	return rest[:idx], rest[idx+len("\n---\n"):], nil
}

func parseSections(body []byte) map[string]string {
	sections := map[string]string{}
	text := string(body)
	for _, name := range help.RequiredSections {
		open := "<" + name + ">"
		close := "</" + name + ">"
		start := strings.Index(text, open)
		end := strings.Index(text, close)
		if start >= 0 && end > start {
			sections[name] = strings.TrimSpace(text[start+len(open) : end])
		}
	}
	return sections
}

func within(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
