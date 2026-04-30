package profile_test

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/ext/adaptive/profile"
)

func TestParentApprovalState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := profile.NewStore(t.TempDir())
	pending, err := s.Propose(ctx, profile.Change{ChildID: "cross", Field: "learning_style", Proposed: "visual"})
	if err != nil || pending.State != profile.Pending {
		t.Fatalf("Propose = %+v err=%v", pending, err)
	}
	if got, _ := s.Read(ctx, "cross"); got.Preferences["learning_style"] != "" {
		t.Fatalf("pending change updated active profile: %+v", got)
	}
	if err := s.Resolve(ctx, pending.ID, profile.Edited, "visual with short examples"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, _ := s.Read(ctx, "cross")
	if got.Preferences["learning_style"] != "visual with short examples" {
		t.Fatalf("active profile = %+v", got)
	}
	changes, _ := s.Changes(ctx, "cross")
	if changes[0].Proposed != "visual" || changes[0].Edited != "visual with short examples" {
		t.Fatalf("edited change lost history: %+v", changes[0])
	}
}

func TestChildDataIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := profile.NewStore(t.TempDir())
	ch, _ := s.Propose(ctx, profile.Change{ChildID: "cross", Field: "math", Proposed: "hands-on"})
	_ = s.Resolve(ctx, ch.ID, profile.Approved, "")
	got, _ := s.Read(ctx, "other")
	if len(got.Preferences) != 0 {
		t.Fatalf("other child saw private data: %+v", got)
	}
}
