package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/tvmaly/nanogo/modules/lesson"
	"github.com/tvmaly/nanogo/modules/media"
)

func (s *Service) registerLessonOps() {
	if s.cfg.Lesson == nil {
		return
	}
	s.registry.Register("lesson.list", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ChildID string `json:"child_id"`
		}
		_ = json.Unmarshal(raw, &req)
		return s.cfg.Lesson.List(ctx, req.ChildID, s.cfg.LessonBundles), nil
	})
	s.registry.Register("lesson.start", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			LessonID string `json:"lesson_id"`
			ChildID  string `json:"child_id"`
		}
		_ = json.Unmarshal(raw, &req)
		for _, b := range s.cfg.LessonBundles {
			if b.ID == req.LessonID {
				return s.cfg.Lesson.Start(ctx, b, req.ChildID)
			}
		}
		return nil, E(CodeInvalidRequest, "lesson not found")
	})
	s.registry.Register("lesson.event", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			SessionID string       `json:"lesson_session_id"`
			Event     lesson.Event `json:"event"`
		}
		_ = json.Unmarshal(raw, &req)
		if err := s.cfg.Lesson.Event(ctx, req.SessionID, req.Event); err != nil {
			return nil, E(CodeUnauthorized, err.Error())
		}
		return map[string]bool{"ok": true}, nil
	})
	s.registry.Register("lesson.advance", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			Session  lesson.Session            `json:"session"`
			Verdicts map[string]lesson.Verdict `json:"verdicts"`
		}
		_ = json.Unmarshal(raw, &req)
		for _, b := range s.cfg.LessonBundles {
			if b.ID == req.Session.LessonID {
				return s.cfg.Lesson.Advance(ctx, req.Session, b, req.Verdicts), nil
			}
		}
		return nil, E(CodeInvalidRequest, "lesson not found")
	})
}

func (s *Service) registerMediaOps() {
	if s.cfg.Media == nil {
		return
	}
	s.registry.Register("media.upload", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var req struct {
			ChildID         string `json:"child_id"`
			LessonID        string `json:"lesson_id"`
			MicroLessonID   string `json:"micro_lesson_id"`
			LessonSessionID string `json:"lesson_session_id"`
			AttemptID       string `json:"attempt_id"`
			ActorRole       string `json:"actor_role"`
			Source          string `json:"source"`
			Filename        string `json:"filename"`
			Nonce           string `json:"nonce"`
			DataBase64      string `json:"data_base64"`
		}
		_ = json.Unmarshal(raw, &req)
		data, err := base64.StdEncoding.DecodeString(req.DataBase64)
		if err != nil {
			return nil, E(CodeInvalidRequest, "data_base64 is invalid")
		}
		meta, err := s.cfg.Media.Upload(ctx, media.UploadRequest{
			ChildID: req.ChildID, LessonID: req.LessonID, MicroLessonID: req.MicroLessonID, LessonSessionID: req.LessonSessionID,
			AttemptID: req.AttemptID, ActorRole: req.ActorRole, Source: req.Source, Filename: req.Filename, Nonce: req.Nonce, Bytes: data,
		})
		if err != nil {
			return nil, E(CodeInvalidRequest, err.Error())
		}
		return meta, nil
	})
}
