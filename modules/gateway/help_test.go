package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/modules/help"
	helpfake "github.com/tvmaly/nanogo/modules/help/fake"
)

func TestHelpOperationsDelegateToHelpService(t *testing.T) {
	fh := &helpfake.Service{
		SearchResp: help.SearchResponse{Hits: []help.SearchHit{{ID: "tools.contracts", Title: "Tools", Summary: "Tool help", Kind: "reference", Snippet: "Tool help"}}},
		TopicResp:  help.TopicResponse{Topic: help.Topic{TopicMeta: help.TopicMeta{ID: "voice.privacy", Title: "Voice Privacy"}, SourcePaths: []string{"ext/voice"}}},
	}
	svc := New(Config{Help: fh})
	payload, err := svc.Dispatch(context.Background(), Request{Method: "help.search", Params: json.RawMessage(`{"query":"tools","limit":3}`)})
	if err != nil {
		t.Fatalf("help.search: %v", err)
	}
	if got := payload.(help.SearchResponse); len(got.Hits) != 1 || got.Hits[0].ID != "tools.contracts" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(fh.SearchCalls) != 1 || fh.SearchCalls[0].Query != "tools" || fh.SearchCalls[0].Limit != 3 {
		t.Fatalf("search calls = %#v", fh.SearchCalls)
	}
	payload, err = svc.Dispatch(context.Background(), Request{Method: "help.topic", Params: json.RawMessage(`{"id":"voice.privacy"}`)})
	if err != nil {
		t.Fatalf("help.topic: %v", err)
	}
	if got := payload.(help.TopicResponse); got.Topic.ID != "voice.privacy" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestUnconfiguredHelpReturnsUnsupported(t *testing.T) {
	svc := New(Config{})
	_, err := svc.Dispatch(context.Background(), Request{Method: "help.search", Params: json.RawMessage(`{"query":"tools"}`)})
	if AsError(err).Code != CodeUnsupported {
		t.Fatalf("code = %v, want %s", AsError(err).Code, CodeUnsupported)
	}
}
