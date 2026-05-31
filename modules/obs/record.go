package obs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const SchemaVersion = "obs.v2"

var (
	ErrInvalidRecord       = errors.New("invalid observation record")
	ErrStoreClosed         = errors.New("observation store closed")
	ErrQueryNotImplemented = errors.New("observation query not implemented")
)

type FailurePolicy string

const (
	FailureBestEffort FailurePolicy = "best_effort"
	FailureFailFast   FailurePolicy = "fail_fast"
)

type ObservationRecord struct {
	SchemaVersion string                     `json:"schema_version"`
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Time          time.Time                  `json:"time"`
	Source        string                     `json:"source,omitempty"`
	Session       string                     `json:"session,omitempty"`
	Turn          int                        `json:"turn,omitempty"`
	Severity      string                     `json:"severity,omitempty"`
	Message       string                     `json:"message,omitempty"`
	Attributes    map[string]any             `json:"attributes,omitempty"`
	Artifacts     []ArtifactRef              `json:"artifacts,omitempty"`
	Error         *ErrorInfo                 `json:"error,omitempty"`
	RepairHints   []RepairHint               `json:"repair_hints,omitempty"`
	Links         []ObservationLink          `json:"links,omitempty"`
	Unknown       map[string]json.RawMessage `json:"-"`
}

type ArtifactRef struct {
	Kind   string            `json:"kind"`
	URI    string            `json:"uri"`
	Digest string            `json:"digest,omitempty"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

type ErrorInfo struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
	Cause   string `json:"cause,omitempty"`
}

type RepairHint struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type ObservationLink struct {
	Rel string `json:"rel"`
	Ref string `json:"ref"`
}

type QuerySpec struct {
	Types   []string
	Session string
	Since   time.Time
	Until   time.Time
	Limit   int
}

type QueryResult struct {
	Records []ObservationRecord
}

type Writer interface {
	Append(context.Context, ObservationRecord) error
}

type Flusher interface {
	Flush() error
}

type Store interface {
	Writer
	Query(context.Context, QuerySpec) (QueryResult, error)
}

func (r ObservationRecord) Validate() error {
	switch {
	case r.SchemaVersion == "":
		return fmt.Errorf("%w: schema_version is required", ErrInvalidRecord)
	case r.SchemaVersion != SchemaVersion:
		return fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidRecord, r.SchemaVersion)
	case r.ID == "":
		return fmt.Errorf("%w: id is required", ErrInvalidRecord)
	case r.Type == "":
		return fmt.Errorf("%w: type is required", ErrInvalidRecord)
	case r.Time.IsZero():
		return fmt.Errorf("%w: time is required", ErrInvalidRecord)
	default:
		return nil
	}
}

func (r ObservationRecord) MarshalJSON() ([]byte, error) {
	type alias ObservationRecord
	base, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	if len(r.Unknown) == 0 {
		return base, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range r.Unknown {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

func (r *ObservationRecord) UnmarshalJSON(data []byte) error {
	type alias ObservationRecord
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, k := range []string{
		"schema_version", "id", "type", "time", "source", "session", "turn", "severity", "message",
		"attributes", "artifacts", "error", "repair_hints", "links",
	} {
		delete(raw, k)
	}
	*r = ObservationRecord(a)
	if len(raw) > 0 {
		r.Unknown = raw
	}
	return nil
}

type FakeStore struct {
	mu      sync.Mutex
	records []ObservationRecord
	closed  bool
}

func NewFakeStore() *FakeStore { return &FakeStore{} }

func (s *FakeStore) Append(_ context.Context, rec ObservationRecord) error {
	if err := rec.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStoreClosed
	}
	s.records = append(s.records, rec)
	return nil
}

func (s *FakeStore) Query(_ context.Context, spec QuerySpec) (QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return QueryResult{}, ErrStoreClosed
	}
	var out []ObservationRecord
	for _, rec := range s.records {
		if !matchesQuery(rec, spec) {
			continue
		}
		out = append(out, rec)
		if spec.Limit > 0 && len(out) >= spec.Limit {
			break
		}
	}
	return QueryResult{Records: out}, nil
}

func (s *FakeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func matchesQuery(rec ObservationRecord, spec QuerySpec) bool {
	if spec.Session != "" && rec.Session != spec.Session {
		return false
	}
	if !spec.Since.IsZero() && rec.Time.Before(spec.Since) {
		return false
	}
	if !spec.Until.IsZero() && rec.Time.After(spec.Until) {
		return false
	}
	if len(spec.Types) > 0 {
		for _, typ := range spec.Types {
			if rec.Type == typ {
				return true
			}
		}
		return false
	}
	return true
}
