package help

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSearchReturnsDeterministicBoundedResults(t *testing.T) {
	svc := testService(t)
	resp, err := svc.Search(context.Background(), SearchRequest{Query: "gateway", Limit: 3, IncludeBody: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Hits) > 3 {
		t.Fatalf("hits = %d", len(resp.Hits))
	}
	if len(resp.Hits) < 2 {
		t.Fatalf("hits = %#v", resp.Hits)
	}
	for _, h := range resp.Hits {
		if h.ID == "" || h.Title == "" || h.Summary == "" || h.Kind == "" || h.Snippet == "" {
			t.Fatalf("incomplete hit: %#v", h)
		}
	}
	for i := 1; i < len(resp.Hits); i++ {
		prev, cur := resp.Hits[i-1], resp.Hits[i]
		if prev.Score < cur.Score || (prev.Score == cur.Score && prev.Title > cur.Title) {
			t.Fatalf("not deterministically ordered: %#v", resp.Hits)
		}
	}
}

func TestTopicLookupReturnsStructuredTopicAndMissingDomainError(t *testing.T) {
	svc := testService(t)
	resp, err := svc.Topic(context.Background(), TopicRequest{ID: "tui.slash_commands"})
	if err != nil {
		t.Fatalf("Topic: %v", err)
	}
	if resp.Topic.Title == "" || len(resp.Topic.SourcePaths) == 0 || len(resp.Topic.Invariants) == 0 || resp.Topic.Sections["verification"] == "" {
		t.Fatalf("topic = %#v", resp.Topic)
	}
	if strings.Contains(resp.Topic.Body, "Voice Privacy") {
		t.Fatalf("topic included unrelated body: %s", resp.Topic.Body)
	}
	_, err = svc.Topic(context.Background(), TopicRequest{ID: "missing.topic"})
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "missing.topic") {
		t.Fatalf("missing err = %v", err)
	}
}

func TestSuggestUsesInterfaceContextAndRenderIsBounded(t *testing.T) {
	svc := testService(t)
	resp, err := svc.Suggest(context.Background(), SuggestRequest{Interface: "tui.chat", Limit: 5})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(resp.Hits) == 0 || resp.Hits[0].ID != "tui.slash_commands" {
		t.Fatalf("suggest = %#v", resp.Hits)
	}
	rendered, err := svc.Render(context.Background(), RenderRequest{TopicID: "quickstart.overview", Format: FormatTUI, Width: 36})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(rendered.Text) > MaxRenderedBytes {
		t.Fatalf("rendered too large")
	}
	for _, line := range strings.Split(rendered.Text, "\n") {
		if len(line) > 45 && !strings.HasPrefix(line, "## ") {
			t.Fatalf("line not wrapped: %q", line)
		}
	}
}

func TestValidateRejectsDuplicateBrokenLinksAndSections(t *testing.T) {
	pack := samplePack()
	pack.Topics = append(pack.Topics, pack.Topics[0])
	if errs := ValidatePack(pack); !containsErr(errs, "duplicate topic id") {
		t.Fatalf("errs = %#v", errs)
	}
	pack = samplePack()
	pack.Topics[0].Related = []string{"missing.topic"}
	if errs := ValidatePack(pack); !containsErr(errs, "missing.topic") {
		t.Fatalf("errs = %#v", errs)
	}
	pack = samplePack()
	delete(pack.Topics[0].Sections, "failure_modes")
	if errs := ValidatePack(pack); !containsErr(errs, "quickstart.overview") || !containsErr(errs, "failure_modes") {
		t.Fatalf("errs = %#v", errs)
	}
}

func testService(t *testing.T) *LocalService {
	t.Helper()
	cat, err := NewCatalog(samplePack())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(cat)
}

func samplePack() Pack {
	sections := func(s string) map[string]string {
		return map[string]string{"rules": "Use local help.", "summary": s, "procedure": "Open help.", "examples": "nanogo help", "failure_modes": "Missing topics return not_found.", "verification": "Run go test."}
	}
	return Pack{SchemaVersion: PackSchemaVersion, PackID: "test", RootTopics: []string{"quickstart.overview"}, Topics: []Topic{
		{SchemaVersion: TopicSchemaVersion, TopicMeta: TopicMeta{ID: "quickstart.overview", Title: "Quickstart Overview", Summary: "Start with local gateway help.", Kind: "guide", Tags: []string{"gateway"}}, SourcePaths: []string{"README.md"}, Invariants: []string{"local_only"}, LastVerified: "2026-05-26", Sections: sections(strings.Repeat("Use the gateway help system. ", 20)), Body: strings.Repeat("Use the gateway help system. ", 20)},
		{SchemaVersion: TopicSchemaVersion, TopicMeta: TopicMeta{ID: "tui.slash_commands", Title: "TUI Slash Commands", Summary: "TUI help and gateway commands.", Kind: "command", Interfaces: []string{"tui.chat", "tui"}, Related: []string{"gateway.operations"}}, SourcePaths: []string{"ext/transport/tui"}, Invariants: []string{"transports_must_use_gateway"}, LastVerified: "2026-05-26", Sections: sections("Use /help."), Body: "Use /help topic voice.privacy."},
		{SchemaVersion: TopicSchemaVersion, TopicMeta: TopicMeta{ID: "gateway.operations", Title: "Gateway Operations", Summary: "Gateway dispatch operations.", Kind: "reference", Interfaces: []string{"gateway"}, Tags: []string{"gateway"}}, SourcePaths: []string{"modules/gateway"}, Invariants: []string{"core_boundary"}, LastVerified: "2026-05-26", Sections: sections("Dispatch help.search."), Body: "Gateway operations include help.search and help.topic."},
		{SchemaVersion: TopicSchemaVersion, TopicMeta: TopicMeta{ID: "voice.privacy", Title: "Voice Privacy", Summary: "Voice privacy help.", Kind: "safety", Tags: []string{"voice"}}, SourcePaths: []string{"ext/voice"}, Invariants: []string{"no_raw_pcm"}, LastVerified: "2026-05-26", Sections: sections("No raw microphone audio."), Body: "Voice privacy."},
	}}
}

func containsErr(errs []string, want string) bool {
	for _, err := range errs {
		if strings.Contains(err, want) {
			return true
		}
	}
	return false
}
