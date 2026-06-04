package agentbrowser

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tvmaly/nanogo/modules/browser"
)

type fakeRunner struct {
	out  []byte
	errb []byte
	err  error
	args [][]string
}

func (r *fakeRunner) Run(_ context.Context, args []string) ([]byte, []byte, error) {
	r.args = append(r.args, append([]string(nil), args...))
	return r.out, r.errb, r.err
}

func TestHealthRequiresMinimumVersion(t *testing.T) {
	r := &fakeRunner{out: []byte("agent-browser 0.26.9")}
	_, err := New(r).Health(context.Background())
	if code(err) != browser.CodeUnsupportedVersion {
		t.Fatalf("code = %v err=%v", code(err), err)
	}
	r = &fakeRunner{out: []byte("agent-browser 0.27.0")}
	health, err := New(r).Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !health.OK || health.Version != "0.27.0" {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func TestStartUsesArgvAndDomainFileFlags(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"success":true,"data":{"launched":true},"error":null}`)}
	sess, err := New(r).Start(context.Background(), browser.StartRequest{
		SessionName: "lesson", Headed: true, AllowedDomains: []string{"example.test"}, FileRoots: []string{"workspace/lessons"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "lesson" || sess.ActiveTabID != "active" {
		t.Fatalf("unexpected session: %+v", sess)
	}
	want := []string{"open", "--json", "--session", "lesson", "--session-name", "lesson", "--headed", "--allowed-domains", "example.test", "--allow-file-access"}
	if !reflect.DeepEqual(r.args[0], want) {
		t.Fatalf("args = %#v want %#v", r.args[0], want)
	}
}

func TestSnapshotNormalizesRefsAndPreservesAdapterRefs(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"success":true,"data":{"snapshot_id":"snap","version":7,"text":"Go","nodes":[{"ref":"ab://42","role":"button","label":"Go"}]},"error":null}`)}
	snap, err := New(r).Snapshot(context.Background(), browser.SnapshotRequest{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Nodes[0].Ref != "ref://v7/e1" || snap.Nodes[0].AdapterRef != "ab://42" {
		t.Fatalf("bad refs: %+v", snap.Nodes[0])
	}
}

func TestSnapshotParsesCurrentAgentBrowserRefsShape(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"success":true,"data":{"origin":"https://example.com/","refs":{"e2":{"name":"Learn more","role":"link"},"e1":{"name":"Example Domain","role":"heading"}},"snapshot":"text"},"error":null}`)}
	snap, err := New(r).Snapshot(context.Background(), browser.SnapshotRequest{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Text != "text" || len(snap.Nodes) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if snap.Nodes[0].Ref != "ref://v1/e1" || snap.Nodes[0].AdapterRef != "@e1" || snap.Nodes[0].Label != "Example Domain" {
		t.Fatalf("bad first node: %+v", snap.Nodes[0])
	}
}

func TestErrorsMapToTypedBrowserErrors(t *testing.T) {
	r := &fakeRunner{err: errors.New("exit 1"), errb: []byte("stale ref")}
	_, err := New(r).Tabs(context.Background(), browser.TabsRequest{SessionID: "s1"})
	if code(err) != browser.CodeStaleRef {
		t.Fatalf("code = %v err=%v", code(err), err)
	}
}

func code(err error) browser.ErrorCode {
	var be *browser.Error
	if errors.As(err, &be) {
		return be.Code
	}
	return ""
}
