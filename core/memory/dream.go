package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/llm"
)

type dream struct {
	store    *Store
	provider llm.Provider
}

func NewDream(store *Store, provider llm.Provider) Dreamer {
	return &dream{store: store, provider: provider}
}

const cursorMarker = "__dream_cursor__"

func (d *dream) Dream(ctx context.Context) error {
	entries, err := d.store.ReadHistory()
	if err != nil {
		return fmt.Errorf("dream: read history: %w", err)
	}
	cursorIdx := -1
	for i, e := range entries {
		if e.Role == cursorMarker {
			cursorIdx = i
		}
	}
	unprocessed := entries[cursorIdx+1:]
	if len(unprocessed) == 0 {
		return nil
	}
	plan, err := d.llmCall(ctx, d.analyzePrompt(unprocessed))
	if err != nil {
		return fmt.Errorf("dream phase1: %w", err)
	}
	if err := d.applyPlan(ctx, plan); err != nil {
		return fmt.Errorf("dream phase2: %w", err)
	}
	return d.store.AppendHistory(HistoryEntry{At: time.Now(), Role: cursorMarker, Cursor: true})
}

func (d *dream) llmCall(ctx context.Context, prompt string) (string, error) {
	ch, err := d.provider.Chat(ctx, llm.Request{
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.TextDelta)
	}
	return strings.TrimSpace(sb.String()), nil
}

// extractJSON finds the first {...} block in s, stripping any markdown fences.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return strings.TrimSpace(s)
}

func (d *dream) analyzePrompt(entries []HistoryEntry) string {
	mem, _ := d.store.ReadFile("MEMORY.md")
	soul, _ := d.store.ReadFile("SOUL.md")
	user, _ := d.store.ReadFile("USER.md")
	var sb strings.Builder
	sb.WriteString("You are a memory curator. Given recent conversation history and current identity files, ")
	sb.WriteString("produce JSON with keys 'MEMORY.md','SOUL.md','USER.md' mapping to updated file content. ")
	sb.WriteString("Only include files needing updates. Respond ONLY with JSON, no fences.\n\n")
	sb.WriteString("## MEMORY.md\n" + mem + "\n## SOUL.md\n" + soul + "\n## USER.md\n" + user + "\n\n## History\n")
	for _, e := range entries {
		sb.WriteString(e.Role + ": " + e.Content + "\n")
	}
	return sb.String()
}

func (d *dream) applyPlan(ctx context.Context, plan string) error {
	mem, _ := d.store.ReadFile("MEMORY.md")
	soul, _ := d.store.ReadFile("SOUL.md")
	user, _ := d.store.ReadFile("USER.md")
	prompt := "Apply this memory update plan:\n" + plan +
		"\n\nCurrent MEMORY.md:\n" + mem + "\nCurrent SOUL.md:\n" + soul + "\nCurrent USER.md:\n" + user +
		"\n\nReturn JSON with keys 'MEMORY.md','SOUL.md','USER.md' containing full updated content. Respond ONLY with valid JSON, no markdown fences."
	result, err := d.llmCall(ctx, prompt)
	if err != nil {
		return err
	}
	raw := extractJSON(result)
	var files map[string]string
	if err := jsonUnmarshal([]byte(raw), &files); err != nil {
		return nil // non-fatal: LLM returned non-JSON
	}
	for name, content := range files {
		if name == "MEMORY.md" || name == "SOUL.md" || name == "USER.md" {
			if err := d.store.WriteFile(name, content); err != nil {
				return err
			}
		}
	}
	return nil
}
