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

func TestHealthRejectsPrereleaseAndAcceptsBuildMetadata(t *testing.T) {
	r := &fakeRunner{out: []byte("agent-browser 0.27.0-rc1")}
	_, err := New(r).Health(context.Background())
	if code(err) != browser.CodeUnsupportedVersion {
		t.Fatalf("prerelease code = %v err=%v", code(err), err)
	}
	r = &fakeRunner{out: []byte("agent-browser 0.27.1+build.123")}
	health, err := New(r).Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if health.Version != "0.27.1" {
		t.Fatalf("version = %q", health.Version)
	}
}

func TestHealthMalformedVersionIsTypedUnavailable(t *testing.T) {
	r := &fakeRunner{out: []byte("agent-browser dev")}
	_, err := New(r).Health(context.Background())
	if code(err) != browser.CodeAdapterUnavailable {
		t.Fatalf("code = %v err=%v", code(err), err)
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

func TestNavigateAllowsFileAccessForFileURLs(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"success":true,"data":{"url":"file:///tmp/index.html","status":"loaded"},"error":null}`)}
	if _, err := New(r).Navigate(context.Background(), browser.NavigateRequest{SessionID: "s1", URL: "file:///tmp/index.html", WaitUntil: "load"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "file:///tmp/index.html", "--json", "--session", "s1", "--wait-until", "load", "--allow-file-access"}
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

func TestScreenshotPassesRequestedArtifactPath(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"success":true,"data":{"path":"/tmp/shot.png"},"error":null}`)}
	artifact, err := New(r).Screenshot(context.Background(), browser.ScreenshotRequest{SessionID: "s1", Path: "/tmp/shot.png", FullPage: true})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Path != "/tmp/shot.png" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	want := []string{"screenshot", "--full", "/tmp/shot.png", "--json", "--session", "s1"}
	if !reflect.DeepEqual(r.args[0], want) {
		t.Fatalf("args = %#v want %#v", r.args[0], want)
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
