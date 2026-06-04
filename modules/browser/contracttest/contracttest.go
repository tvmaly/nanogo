package contracttest

import (
	"context"
	"testing"

	"github.com/tvmaly/nanogo/modules/browser"
)

func RunControllerContract(t *testing.T, makeController func() browser.Controller) {
	t.Helper()
	ctx := context.Background()
	c := makeController()
	health, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !health.OK {
		t.Fatalf("health ok=false")
	}
	sess, err := c.Start(ctx, browser.StartRequest{SessionName: "contract", Headed: false})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	page, err := c.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: "https://example.test/lesson", WaitUntil: "load"})
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if page.TabID == "" || page.URL == "" {
		t.Fatalf("navigate returned incomplete page: %+v", page)
	}
	snap, err := c.Snapshot(ctx, browser.SnapshotRequest{SessionID: sess.ID, InteractiveOnly: true, MaxDepth: 8, MaxOutputBytes: 65536})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snap.Nodes) == 0 || snap.Nodes[0].Ref == "" || snap.Nodes[0].AdapterRef == "" {
		t.Fatalf("snapshot did not expose normalized refs and adapter refs: %+v", snap.Nodes)
	}
	if _, err := c.Act(ctx, browser.ActionRequest{SessionID: sess.ID, Kind: browser.ActionClick, Target: browser.Target{Ref: snap.Nodes[0].Ref}}); err != nil {
		t.Fatalf("act: %v", err)
	}
	if err := c.Close(ctx, browser.CloseRequest{SessionID: sess.ID, CloseSession: true}); err != nil {
		t.Fatalf("close: %v", err)
	}
}
