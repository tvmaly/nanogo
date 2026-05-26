package fake

import (
	"context"

	"github.com/tvmaly/nanogo/modules/help"
)

type Service struct {
	SearchCalls  []help.SearchRequest
	TopicCalls   []help.TopicRequest
	SuggestCalls []help.SuggestRequest
	RenderCalls  []help.RenderRequest
	SearchResp   help.SearchResponse
	TopicResp    help.TopicResponse
	SuggestResp  help.SuggestResponse
	RenderResp   help.RenderResponse
	ValidateResp help.ValidateResponse
	Err          error
}

func (s *Service) Search(_ context.Context, req help.SearchRequest) (help.SearchResponse, error) {
	s.SearchCalls = append(s.SearchCalls, req)
	return s.SearchResp, s.Err
}

func (s *Service) Topic(_ context.Context, req help.TopicRequest) (help.TopicResponse, error) {
	s.TopicCalls = append(s.TopicCalls, req)
	return s.TopicResp, s.Err
}

func (s *Service) Suggest(_ context.Context, req help.SuggestRequest) (help.SuggestResponse, error) {
	s.SuggestCalls = append(s.SuggestCalls, req)
	return s.SuggestResp, s.Err
}

func (s *Service) Render(_ context.Context, req help.RenderRequest) (help.RenderResponse, error) {
	s.RenderCalls = append(s.RenderCalls, req)
	return s.RenderResp, s.Err
}

func (s *Service) Validate(context.Context, help.ValidateRequest) (help.ValidateResponse, error) {
	return s.ValidateResp, s.Err
}
