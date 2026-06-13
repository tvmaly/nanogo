// Package media stores local capture metadata and privacy-safe references.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	CaptureSchema    = "media.capture.v1"
	TombstoneSchema  = "media.tombstone.v1"
	DefaultRetention = "keep_until_parent_deletes"
)

type Config struct {
	Root           string
	Enabled        bool
	MaxUploadBytes int64
	Now            func() time.Time
	ValidateNonce  func(sessionID, nonce string) bool
}

type Store struct {
	cfg Config
}

type UploadRequest struct {
	ChildID         string
	LessonID        string
	MicroLessonID   string
	LessonSessionID string
	AttemptID       string
	ActorRole       string
	Source          string
	Filename        string
	Nonce           string
	Bytes           []byte
}

type CaptureMeta struct {
	SchemaVersion   string    `json:"schema_version"`
	ChildID         string    `json:"child_id"`
	LessonID        string    `json:"lesson_id"`
	MicroLessonID   string    `json:"micro_lesson_id"`
	LessonSessionID string    `json:"lesson_session_id"`
	AttemptID       string    `json:"attempt_id"`
	ActorRole       string    `json:"actor_role"`
	Source          string    `json:"source"`
	Filename        string    `json:"filename"`
	SHA256          string    `json:"sha256"`
	SizeBytes       int64     `json:"size_bytes"`
	Retention       string    `json:"retention"`
	CreatedAt       time.Time `json:"created_at"`
	Path            string    `json:"path"`
}

type Tombstone struct {
	SchemaVersion string    `json:"schema_version"`
	ChildID       string    `json:"child_id"`
	LessonID      string    `json:"lesson_id"`
	MicroLessonID string    `json:"micro_lesson_id"`
	AttemptID     string    `json:"attempt_id"`
	DeletedAt     time.Time `json:"deleted_at"`
	Reason        string    `json:"reason"`
}

func New(cfg Config) *Store {
	if cfg.Root == "" {
		cfg.Root = "."
	}
	if cfg.MaxUploadBytes == 0 {
		cfg.MaxUploadBytes = 25 << 20
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Store{cfg: cfg}
}

func (s *Store) Upload(_ context.Context, req UploadRequest) (CaptureMeta, error) {
	if !s.cfg.Enabled {
		return CaptureMeta{}, errors.New("media is disabled")
	}
	if s.cfg.ValidateNonce != nil && !s.cfg.ValidateNonce(req.LessonSessionID, req.Nonce) {
		return CaptureMeta{}, errors.New("nonce validation failed")
	}
	if int64(len(req.Bytes)) > s.cfg.MaxUploadBytes {
		return CaptureMeta{}, fmt.Errorf("upload exceeds max_upload_bytes")
	}
	if req.ActorRole == "" {
		req.ActorRole = "student"
	}
	if req.Source == "" {
		req.Source = "webcam"
	}
	if req.Filename == "" {
		req.Filename = "capture.webm"
	}
	base := filepath.Join(s.cfg.Root, "memory", "media", safe(req.ChildID), safe(req.LessonID), safe(req.MicroLessonID), safe(req.AttemptID))
	if err := os.MkdirAll(base, 0755); err != nil {
		return CaptureMeta{}, err
	}
	capturePath := filepath.Join(base, req.Filename)
	if err := os.WriteFile(capturePath, req.Bytes, 0600); err != nil {
		return CaptureMeta{}, err
	}
	sum := sha256.Sum256(req.Bytes)
	meta := CaptureMeta{
		SchemaVersion: CaptureSchema, ChildID: req.ChildID, LessonID: req.LessonID, MicroLessonID: req.MicroLessonID,
		LessonSessionID: req.LessonSessionID, AttemptID: req.AttemptID, ActorRole: req.ActorRole, Source: req.Source,
		Filename: req.Filename, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(req.Bytes)),
		Retention: DefaultRetention, CreatedAt: s.cfg.Now(), Path: capturePath,
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(base, "meta.json"), data, 0600); err != nil {
		return CaptureMeta{}, err
	}
	return meta, nil
}

func (s *Store) Delete(_ context.Context, meta CaptureMeta, reason string) error {
	base := filepath.Dir(meta.Path)
	_ = os.Remove(meta.Path)
	_ = os.Remove(filepath.Join(base, "meta.json"))
	t := Tombstone{SchemaVersion: TombstoneSchema, ChildID: meta.ChildID, LessonID: meta.LessonID, MicroLessonID: meta.MicroLessonID, AttemptID: meta.AttemptID, DeletedAt: s.cfg.Now(), Reason: reason}
	data, _ := json.MarshalIndent(t, "", "  ")
	return os.WriteFile(filepath.Join(base, "tombstone.json"), data, 0600)
}

func PrivacySafeRef(meta CaptureMeta) map[string]string {
	return map[string]string{
		"schema_version":  meta.SchemaVersion,
		"child_id":        meta.ChildID,
		"lesson_id":       meta.LessonID,
		"micro_lesson_id": meta.MicroLessonID,
		"attempt_id":      meta.AttemptID,
		"sha256":          meta.SHA256,
		"retention":       meta.Retention,
	}
}

func safe(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}
