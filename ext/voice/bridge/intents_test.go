package bridge

import (
	"context"
	"testing"
)

func TestVoiceBridgeParentLessonRequest(t *testing.T) {
	flow := &fakeFlow{}
	b := New(flow)
	text, action, err := b.Handle(context.Background(), Intent{Type: "parent_create_lesson_request", ChildID: "cross", Topic: "magnets", Text: "create a lesson"})
	if err != nil {
		t.Fatal(err)
	}
	if action != nil || text == "" {
		t.Fatalf("text=%q action=%#v", text, action)
	}
	if flow.got.Type != "parent_create_lesson_request" || flow.got.ChildID != "cross" {
		t.Fatalf("intent = %#v", flow.got)
	}
}

func TestVoiceBridgeChildHelpRequest(t *testing.T) {
	flow := &fakeFlow{}
	b := New(flow)
	if _, _, err := b.Handle(context.Background(), Intent{Type: "child_help_request", ChildID: "cross", Subject: "science"}); err != nil {
		t.Fatal(err)
	}
	if flow.got.Type != "child_help_request" || flow.got.Subject != "science" {
		t.Fatalf("intent = %#v", flow.got)
	}
}

func TestVoiceBridgeWebUIAction(t *testing.T) {
	b := New(nil)
	_, action, err := b.Handle(context.Background(), Intent{Type: "child_show_video_request", ChildID: "cross", Topic: "magnets"})
	if err != nil {
		t.Fatal(err)
	}
	if action == nil || action.Type != "child_show_video_request" || action.ChildID != "cross" {
		t.Fatalf("action = %#v", action)
	}
}

type fakeFlow struct {
	got Intent
}

func (f *fakeFlow) Submit(_ context.Context, intent Intent) (string, error) {
	f.got = intent
	return "I will help with that.", nil
}
