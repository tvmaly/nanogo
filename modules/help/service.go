package help

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

type LocalService struct {
	catalog Catalog
}

func NewService(c Catalog) *LocalService { return &LocalService{catalog: c} }

func (s *LocalService) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	metas, err := s.catalog.ListTopics(ctx)
	if err != nil {
		return SearchResponse{}, err
	}
	q := normalize(req.Query)
	limit := safeLimit(req.Limit)
	var hits []SearchHit
	for _, meta := range metas {
		score := scoreMeta(q, req, meta)
		var topic Topic
		if score == 0 || req.IncludeBody {
			topic, _ = s.catalog.GetTopic(ctx, meta.ID)
			bodyScore := scoreBody(q, topic)
			score += bodyScore
		}
		if q == "" && req.Interface != "" && contains(meta.Interfaces, req.Interface) {
			score += 10
		}
		if score <= 0 && q != "" {
			continue
		}
		if topic.ID == "" {
			topic, _ = s.catalog.GetTopic(ctx, meta.ID)
		}
		hits = append(hits, SearchHit{
			ID: meta.ID, Title: meta.Title, Summary: meta.Summary, Kind: meta.Kind,
			Score: score, Tags: append([]string(nil), meta.Tags...), Related: append([]string(nil), meta.Related...),
			Snippet: snippet(q, topic),
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Title != hits[j].Title {
			return hits[i].Title < hits[j].Title
		}
		return hits[i].ID < hits[j].ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return SearchResponse{Query: q, Hits: hits}, nil
}

func (s *LocalService) Topic(ctx context.Context, req TopicRequest) (TopicResponse, error) {
	t, err := s.catalog.GetTopic(ctx, req.ID)
	if err != nil {
		return TopicResponse{}, err
	}
	return TopicResponse{Topic: t}, nil
}

func (s *LocalService) Suggest(ctx context.Context, req SuggestRequest) (SuggestResponse, error) {
	q := strings.ReplaceAll(req.Interface, ".", " ")
	resp, err := s.Search(ctx, SearchRequest{Query: q, Interface: req.Interface, Limit: req.Limit})
	if err != nil {
		return SuggestResponse{}, err
	}
	return SuggestResponse{Hits: resp.Hits}, nil
}

func (s *LocalService) Render(ctx context.Context, req RenderRequest) (RenderResponse, error) {
	if req.Format == "" {
		req.Format = FormatMarkdown
	}
	t, err := s.catalog.GetTopic(ctx, req.TopicID)
	if err != nil {
		return RenderResponse{}, err
	}
	var text string
	if req.Format == FormatJSON {
		b, _ := json.MarshalIndent(t, "", "  ")
		text = string(b)
	} else {
		text = renderTopic(t, req.Format, req.Width)
	}
	if len(text) > MaxRenderedBytes {
		text = text[:MaxRenderedBytes]
	}
	return RenderResponse{TopicID: t.ID, Format: req.Format, Text: text}, nil
}

func (s *LocalService) Validate(ctx context.Context, _ ValidateRequest) (ValidateResponse, error) {
	metas, err := s.catalog.ListTopics(ctx)
	if err != nil {
		return ValidateResponse{}, err
	}
	pack := Pack{SchemaVersion: PackSchemaVersion}
	for _, meta := range metas {
		t, err := s.catalog.GetTopic(ctx, meta.ID)
		if err != nil {
			return ValidateResponse{}, err
		}
		pack.Topics = append(pack.Topics, t)
	}
	errs := ValidatePack(pack)
	return ValidateResponse{OK: len(errs) == 0, Errors: errs}, nil
}

func safeLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

func normalize(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func scoreMeta(q string, req SearchRequest, meta TopicMeta) int {
	score := 0
	if q == "" {
		score = 1
	}
	fields := []struct {
		text   string
		points int
	}{
		{meta.ID, 100}, {meta.Title, 80}, {meta.Summary, 40}, {meta.Kind, 20},
		{strings.Join(meta.Tags, " "), 25}, {strings.Join(meta.Interfaces, " "), 20}, {strings.Join(meta.Audiences, " "), 10},
	}
	for _, f := range fields {
		text := normalize(f.text)
		if q != "" && text == q {
			score += f.points * 2
		} else if q != "" && strings.Contains(text, q) {
			score += f.points
		}
		for _, tok := range strings.Fields(q) {
			if strings.Contains(text, tok) {
				score += f.points / 4
			}
		}
	}
	if req.Interface != "" && contains(meta.Interfaces, req.Interface) {
		score += 15
	}
	if req.Audience != "" && contains(meta.Audiences, req.Audience) {
		score += 10
	}
	return score
}

func scoreBody(q string, t Topic) int {
	if q == "" || t.ID == "" {
		return 0
	}
	body := normalize(t.Body + " " + strings.Join(sectionValues(t.Sections), " "))
	score := 0
	if strings.Contains(body, q) {
		score += 15
	}
	for _, tok := range strings.Fields(q) {
		if strings.Contains(body, tok) {
			score += 3
		}
	}
	return score
}

func sectionValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func snippet(q string, t Topic) string {
	text := t.Summary
	if q != "" {
		body := t.Body + "\n" + strings.Join(sectionValues(t.Sections), "\n")
		idx := strings.Index(normalize(body), q)
		if idx >= 0 {
			text = body
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > MaxSnippet {
		return text[:MaxSnippet]
	}
	return text
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
