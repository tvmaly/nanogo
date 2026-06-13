package media_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/modules/media"
)

func TestUploadRoundTripMetadataRetentionAndRedaction(t *testing.T) {
	root := t.TempDir()
	store := media.New(media.Config{Root: root, Enabled: true, MaxUploadBytes: 10 << 20, Now: fixed, ValidateNonce: func(_, nonce string) bool { return nonce == "nonce-1" }})
	meta, err := store.Upload(context.Background(), media.UploadRequest{
		ChildID: "cross", LessonID: "lesson-yoyo", MicroLessonID: "ml-01-the-throw", LessonSessionID: "ls-1",
		AttemptID: "1", Source: "webcam", Filename: "capture.webm", Nonce: "nonce-1", Bytes: bytes.Repeat([]byte{1}, 5<<20),
	})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "memory", "media", "cross", "lesson-yoyo", "ml-01-the-throw", "1")
	if _, err := os.Stat(filepath.Join(base, "capture.webm")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(base, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"media.capture.v1", "webcam", "sha256", "keep_until_parent_deletes", "lesson_session_id"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("meta missing %q: %s", want, data)
		}
	}
	ref := media.PrivacySafeRef(meta)
	if _, ok := ref["path"]; ok {
		t.Fatalf("privacy ref leaked path: %+v", ref)
	}
}

func TestUploadRejectsSizeCapAndInvalidNonceWithoutWriting(t *testing.T) {
	root := t.TempDir()
	store := media.New(media.Config{Root: root, Enabled: true, MaxUploadBytes: 1024, ValidateNonce: func(_, nonce string) bool { return nonce == "ok" }})
	_, err := store.Upload(context.Background(), media.UploadRequest{ChildID: "cross", LessonID: "lesson", MicroLessonID: "ml", AttemptID: "1", Nonce: "bad", Bytes: []byte{1}})
	if err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce error, got %v", err)
	}
	_, err = store.Upload(context.Background(), media.UploadRequest{ChildID: "cross", LessonID: "lesson", MicroLessonID: "ml", AttemptID: "1", Nonce: "ok", Bytes: bytes.Repeat([]byte{1}, 2048)})
	if err == nil || !strings.Contains(err.Error(), "max_upload_bytes") {
		t.Fatalf("expected size cap error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "memory", "media")); !os.IsNotExist(err) {
		t.Fatalf("media dir should not exist after rejected upload: %v", err)
	}
}

func TestDeleteWritesTombstone(t *testing.T) {
	root := t.TempDir()
	store := media.New(media.Config{Root: root, Enabled: true, Now: fixed})
	meta, err := store.Upload(context.Background(), media.UploadRequest{ChildID: "cross", LessonID: "lesson", MicroLessonID: "ml", AttemptID: "1", Bytes: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), meta, "parent request"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(meta.Path), "tombstone.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "media.tombstone.v1") || !strings.Contains(string(data), "parent request") {
		t.Fatalf("bad tombstone: %s", data)
	}
}

func fixed() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
