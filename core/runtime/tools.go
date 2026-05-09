// Package runtime contains small runtime composition helpers.
package runtime

import (
	"context"
	"fmt"
	"sort"

	"github.com/tvmaly/nanogo/core/tools"
)

// NamedSource optionally identifies a Source in duplicate-name diagnostics.
type NamedSource interface {
	SourceName() string
}

type multiSource struct{ sources []tools.Source }

// NewMultiSource returns a Source that merges child sources deterministically.
func NewMultiSource(sources ...tools.Source) tools.Source {
	return &multiSource{sources: sources}
}

func (m *multiSource) Tools(ctx context.Context, turn tools.TurnInfo) ([]tools.Tool, error) {
	byName := map[string]tools.Tool{}
	owners := map[string]string{}
	for i, src := range m.sources {
		if src == nil {
			continue
		}
		list, err := src.Tools(ctx, turn)
		if err != nil {
			return nil, err
		}
		owner := fmt.Sprintf("source[%d]", i)
		if named, ok := src.(NamedSource); ok && named.SourceName() != "" {
			owner = named.SourceName()
		}
		for _, tool := range list {
			name := tool.Name()
			if _, exists := byName[name]; exists {
				return nil, fmt.Errorf("duplicate tool name %q from %s and %s", name, owners[name], owner)
			}
			byName[name] = tool
			owners[name] = owner
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]tools.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out, nil
}
