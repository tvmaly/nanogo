package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type topicMeta struct {
	Slug       string
	LastAccess time.Time
	RefCount   int
}

type curator struct {
	store *Store
	clock func() time.Time
}

func NewCurator(store *Store) CuratorStore {
	return &curator{store: store, clock: time.Now}
}

func NewCuratorWithClock(store *Store, clock func() time.Time) CuratorStore {
	return &curator{store: store, clock: clock}
}

func (c *curator) topicRel(slug string) string {
	return "memory/topics/" + slug + ".md"
}

func (c *curator) header() string {
	return "<!-- la:" + c.clock().Format(time.RFC3339) + " rc:1 -->\n"
}

func (c *curator) WriteTopic(slug, content string) error {
	return c.store.WriteFile(c.topicRel(slug), c.header()+content)
}

func (c *curator) ReadTopic(slug string) (string, error) {
	return c.store.ReadFile(c.topicRel(slug))
}

func (c *curator) EditTopic(slug, old, replacement string) error {
	content, err := c.store.ReadFile(c.topicRel(slug))
	if err != nil {
		return err
	}
	if !strings.Contains(content, old) {
		return fmt.Errorf("curator: edit_topic: string not found in %s", slug)
	}
	return c.store.WriteFile(c.topicRel(slug), strings.Replace(content, old, replacement, 1))
}

func (c *curator) LinkTopics(from, to, relation string) error {
	link := "\n[" + relation + "]: " + to + "\n"
	fc, err := c.store.ReadFile(c.topicRel(from))
	if err != nil {
		return err
	}
	if err := c.store.WriteFile(c.topicRel(from), fc+link); err != nil {
		return err
	}
	tc, err := c.store.ReadFile(c.topicRel(to))
	if err != nil {
		return err
	}
	return c.store.WriteFile(c.topicRel(to), tc+"\n["+relation+"]: "+from+"\n")
}

func (c *curator) Grep(pattern string) ([]string, error) {
	dir := filepath.Join(c.store.Root(), "memory", "topics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var results []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		content, _ := c.store.ReadFile(c.topicRel(slug))
		for i, line := range strings.Split(content, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
				results = append(results, fmt.Sprintf("%s:%d: %s", slug, i+1, line))
			}
		}
	}
	return results, nil
}

func (c *curator) PruneOld(_ context.Context, pruneDays int, threshold float64) error {
	dir := filepath.Join(c.store.Root(), "memory", "topics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := c.clock().AddDate(0, 0, -pruneDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		meta := c.parseMeta(slug)
		if meta.LastAccess.Before(cutoff) && c.importance(meta) < threshold {
			src := filepath.Join(c.store.Root(), c.topicRel(slug))
			dst := filepath.Join(c.store.Root(), "memory", "archive", slug+".md")
			_ = os.Rename(src, dst)
		}
	}
	return nil
}

func (c *curator) parseMeta(slug string) topicMeta {
	m := topicMeta{Slug: slug, RefCount: 1, LastAccess: c.clock()}
	content, _ := c.store.ReadFile(c.topicRel(slug))
	first := strings.SplitN(content, "\n", 2)[0]
	// header: <!-- la:<RFC3339> rc:<n> -->
	if i := strings.Index(first, "la:"); i >= 0 {
		rest := first[i+3:]
		if j := strings.Index(rest, " "); j >= 0 {
			if t, err := time.Parse(time.RFC3339, rest[:j]); err == nil {
				m.LastAccess = t
			}
		}
	}
	return m
}

func (c *curator) importance(m topicMeta) float64 {
	age := c.clock().Sub(m.LastAccess).Hours() / 24
	return math.Exp(-age/30) * float64(m.RefCount)
}

func (c *curator) RebuildIndex() error {
	dir := filepath.Join(c.store.Root(), "memory", "topics")
	entries, _ := os.ReadDir(dir)
	var slugs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			slugs = append(slugs, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(slugs)
	var sb strings.Builder
	sb.WriteString("# Memory Index\n\n")
	for _, slug := range slugs {
		sb.WriteString("- [" + slug + "](memory/topics/" + slug + ".md)\n")
	}
	index := sb.String()
	lines := strings.Split(index, "\n")
	if len(lines) > 200 {
		index = strings.Join(lines[:200], "\n")
	}
	return c.store.WriteFile("index.md", index)
}
