package help

import (
	"fmt"
	"sort"
	"strings"
)

func renderTopic(t Topic, format RenderFormat, width int) string {
	var b strings.Builder
	if format == FormatTUI || format == FormatPlain {
		b.WriteString(t.Title + "\n")
		b.WriteString(t.ID + "\n\n")
	} else {
		b.WriteString("# " + t.Title + "\n\n")
		b.WriteString("Topic: `" + t.ID + "`\n\n")
	}
	b.WriteString(t.Summary + "\n\n")
	if len(t.Related) > 0 {
		b.WriteString("Related: " + strings.Join(t.Related, ", ") + "\n")
	}
	if len(t.SourcePaths) > 0 {
		b.WriteString("Source paths: " + strings.Join(t.SourcePaths, ", ") + "\n")
	}
	if len(t.Invariants) > 0 {
		b.WriteString("Invariants: " + strings.Join(t.Invariants, ", ") + "\n")
	}
	if len(t.Related) > 0 || len(t.SourcePaths) > 0 || len(t.Invariants) > 0 {
		b.WriteString("\n")
	}
	keys := make([]string, 0, len(t.Sections))
	for k := range t.Sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.TrimSpace(t.Sections[k]) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n%s\n\n", k, strings.TrimSpace(t.Sections[k])))
	}
	return wrap(b.String(), width)
}

func wrap(text string, width int) string {
	if width <= 0 || width > 120 {
		return text
	}
	var out strings.Builder
	for _, para := range strings.Split(text, "\n") {
		if len(para) <= width || strings.HasPrefix(para, "## ") || strings.HasPrefix(para, "# ") {
			out.WriteString(para + "\n")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out.WriteString(line + "\n")
				line = word
			} else {
				line += " " + word
			}
		}
		if line != "" {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}
