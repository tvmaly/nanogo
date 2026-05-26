// Package help provides deterministic local help contracts and behavior.
package help

import (
	"context"
	"errors"
	"fmt"
)

const (
	PackSchemaVersion  = "help-pack/v1"
	TopicSchemaVersion = "help-topic/v1"
	DefaultLimit       = 5
	MaxLimit           = 20
	MaxSnippet         = 160
	MaxRenderedBytes   = 8192
)

var ErrNotFound = errors.New("help topic not found")

type NotFoundError struct{ ID string }

func (e NotFoundError) Error() string        { return fmt.Sprintf("help topic %q not found", e.ID) }
func (e NotFoundError) Is(target error) bool { return target == ErrNotFound }

type Service interface {
	Search(context.Context, SearchRequest) (SearchResponse, error)
	Topic(context.Context, TopicRequest) (TopicResponse, error)
	Suggest(context.Context, SuggestRequest) (SuggestResponse, error)
	Render(context.Context, RenderRequest) (RenderResponse, error)
	Validate(context.Context, ValidateRequest) (ValidateResponse, error)
}

type Catalog interface {
	ListTopics(context.Context) ([]TopicMeta, error)
	GetTopic(context.Context, string) (Topic, error)
	RootTopics() []string
}

type Pack struct {
	SchemaVersion string
	PackID        string
	Name          string
	Version       int
	RootTopics    []string
	Topics        []Topic
}

type TopicMeta struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Kind       string   `json:"kind"`
	Interfaces []string `json:"interfaces,omitempty"`
	Audiences  []string `json:"audiences,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Related    []string `json:"related,omitempty"`
}

type Topic struct {
	TopicMeta
	SchemaVersion string            `json:"schema_version"`
	SourcePaths   []string          `json:"source_paths,omitempty"`
	Invariants    []string          `json:"invariants,omitempty"`
	LastVerified  string            `json:"last_verified"`
	Sections      map[string]string `json:"sections"`
	Body          string            `json:"body,omitempty"`
}

type SearchRequest struct {
	Query       string `json:"query"`
	Interface   string `json:"interface,omitempty"`
	Audience    string `json:"audience,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	IncludeBody bool   `json:"include_body,omitempty"`
}

type SearchResponse struct {
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
}

type SearchHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Kind    string   `json:"kind"`
	Score   int      `json:"score"`
	Tags    []string `json:"tags,omitempty"`
	Related []string `json:"related,omitempty"`
	Snippet string   `json:"snippet"`
}

type TopicRequest struct {
	ID        string `json:"id"`
	Interface string `json:"interface,omitempty"`
	Audience  string `json:"audience,omitempty"`
}

type TopicResponse struct {
	Topic Topic `json:"topic"`
}

type SuggestRequest struct {
	Interface string            `json:"interface,omitempty"`
	Context   map[string]string `json:"context,omitempty"`
	Limit     int               `json:"limit,omitempty"`
}

type SuggestResponse struct {
	Hits []SearchHit `json:"hits"`
}

type RenderFormat string

const (
	FormatMarkdown RenderFormat = "markdown"
	FormatPlain    RenderFormat = "plain"
	FormatJSON     RenderFormat = "json"
	FormatTUI      RenderFormat = "tui"
)

type RenderRequest struct {
	TopicID string       `json:"topic_id"`
	Format  RenderFormat `json:"format,omitempty"`
	Width   int          `json:"width,omitempty"`
}

type RenderResponse struct {
	TopicID string       `json:"topic_id"`
	Format  RenderFormat `json:"format"`
	Text    string       `json:"text"`
}

type ValidateRequest struct{}

type ValidateResponse struct {
	OK     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
}
