package gateway_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/lesson"
	"github.com/tvmaly/nanogo/modules/media"
)

func TestLessonAndMediaGatewayOps(t *testing.T) {
	root := t.TempDir()
	lessonSvc := lesson.New(lesson.Config{Root: root, Nonce: func() string { return "nonce-1" }})
	mediaStore := media.New(media.Config{Root: root, Enabled: true, ValidateNonce: func(_, nonce string) bool { return nonce == "nonce-1" }})
	bundle := lesson.Bundle{ID: "lesson-yoyo", ChildID: "cross", Approved: true, Promoted: true, Assigned: true, MicroLessons: []lesson.MicroLesson{{ID: "ml-01", ObjectiveType: lesson.ObjectivePhysicalSkill, LearningEvidence: lesson.LearningEvidence{Requires: []lesson.EvidenceKind{lesson.EvidencePhysicalPerformance}}}}}
	svc := gateway.New(gateway.Config{Lesson: lessonSvc, LessonBundles: []lesson.Bundle{bundle}, Media: mediaStore})

	list, err := svc.Dispatch(context.Background(), gateway.Request{Method: "lesson.list", Params: raw(map[string]string{"child_id": "cross"})})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.([]lesson.Bundle)) != 1 {
		t.Fatalf("list = %+v", list)
	}
	startAny, err := svc.Dispatch(context.Background(), gateway.Request{Method: "lesson.start", Params: raw(map[string]string{"lesson_id": "lesson-yoyo", "child_id": "cross"})})
	if err != nil {
		t.Fatal(err)
	}
	sess := startAny.(lesson.Session)
	_, err = svc.Dispatch(context.Background(), gateway.Request{Method: "lesson.event", Params: raw(map[string]any{"lesson_session_id": sess.ID, "event": lesson.Event{Type: "progress", Nonce: "bad"}})})
	if err == nil {
		t.Fatal("expected nonce error")
	}
	upload, err := svc.Dispatch(context.Background(), gateway.Request{Method: "media.upload", Params: raw(map[string]string{
		"child_id": "cross", "lesson_id": "lesson-yoyo", "micro_lesson_id": "ml-01", "lesson_session_id": sess.ID,
		"attempt_id": "1", "nonce": "nonce-1", "filename": "capture.webm", "data_base64": base64.StdEncoding.EncodeToString([]byte("webm")),
	})})
	if err != nil {
		t.Fatal(err)
	}
	if upload.(media.CaptureMeta).SchemaVersion != media.CaptureSchema {
		t.Fatalf("upload = %+v", upload)
	}
}

func raw(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
