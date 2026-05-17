package evolve

import "sort"

// EditCandidate describes one possible self-evolve edit target.
type EditCandidate struct {
	Path string
}

// RankEditCandidates returns candidates ordered by the Phase 17 self-evolve
// preference: workspace assets first, extension code second, kernel code last.
func RankEditCandidates(candidates []EditCandidate) []EditCandidate {
	out := append([]EditCandidate(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		return editPriority(out[i].Path) < editPriority(out[j].Path)
	})
	return out
}

func editPriority(path string) int {
	switch {
	case IsBlocked(path):
		return 3
	case hasPrefix(path, "workspace/"):
		return 0
	case hasPrefix(path, "ext/"), hasPrefix(path, "modules/"):
		return 1
	default:
		return 2
	}
}

func hasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}
