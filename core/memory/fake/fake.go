// Package fake provides test doubles for core/memory interfaces.
package fake

import (
	"context"
	"sync"
)

// Consolidator is a fake Consolidator that records Run calls.
type Consolidator struct {
	mu      sync.Mutex
	Running bool
}

func (f *Consolidator) Run(ctx context.Context) error {
	f.mu.Lock()
	f.Running = true
	f.mu.Unlock()
	<-ctx.Done()
	return nil
}

// Dreamer is a fake Dreamer.
type Dreamer struct {
	mu      sync.Mutex
	Calls   int
	DreamFn func(ctx context.Context) error
}

func (f *Dreamer) Dream(ctx context.Context) error {
	f.mu.Lock()
	f.Calls++
	fn := f.DreamFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}

// CuratorStore is a fake CuratorStore backed by a simple map.
type CuratorStore struct {
	mu     sync.Mutex
	Topics map[string]string
	Index  string
}

func NewCuratorStore() *CuratorStore {
	return &CuratorStore{Topics: map[string]string{}}
}

func (f *CuratorStore) WriteTopic(slug, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Topics[slug] = content
	return nil
}

func (f *CuratorStore) ReadTopic(slug string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Topics[slug], nil
}

func (f *CuratorStore) EditTopic(slug, old, replacement string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	content := f.Topics[slug]
	f.Topics[slug] = replaceFirst(content, old, replacement)
	return nil
}

func replaceFirst(s, old, new string) string {
	idx := indexOf(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, sub string) int {
	for i := range s {
		if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func (f *CuratorStore) LinkTopics(from, to, relation string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Topics[from] += "\n[" + relation + "]: " + to
	f.Topics[to] += "\n[" + relation + "]: " + from
	return nil
}

func (f *CuratorStore) Grep(pattern string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var results []string
	for slug, content := range f.Topics {
		for i, line := range splitLines(content) {
			if contains(line, pattern) {
				results = append(results, slug+":"+itoa(i+1)+": "+line)
			}
		}
	}
	return results, nil
}

func (f *CuratorStore) PruneOld(_ context.Context, _ int, _ float64) error { return nil }
func (f *CuratorStore) RebuildIndex() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Index = "# Index\n"
	for slug := range f.Topics {
		f.Index += "- " + slug + "\n"
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
