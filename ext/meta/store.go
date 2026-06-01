package meta

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type JSONLStore struct {
	workspace string
	mu        sync.Mutex
}

func NewJSONLStore(workspace string) *JSONLStore {
	if workspace == "" {
		workspace = "."
	}
	return &JSONLStore{workspace: workspace}
}

func (s *JSONLStore) AppendLineage(_ context.Context, rec LineageRecord) error {
	rec.SchemaVersion = nonempty(rec.SchemaVersion, RecordSchema)
	if rec.ID == "" {
		rec.ID = stableRecordID("lineage", rec.Event, rec.RunID, rec.LessonID, rec.Timestamp)
	}
	return s.append("memory/meta/lineage.jsonl", rec)
}

func (s *JSONLStore) AppendGraph(_ context.Context, edge GraphEdge) error {
	edge.SchemaVersion = nonempty(edge.SchemaVersion, RecordSchema)
	if edge.ID == "" {
		edge.ID = stableRecordID("graph", edge.Relation, edge.From, edge.To, edge.Timestamp)
	}
	return s.append("memory/meta/artifact_graph.jsonl", edge)
}

func (s *JSONLStore) AppendRun(_ context.Context, run ExperimentRun) error {
	run.SchemaVersion = nonempty(run.SchemaVersion, RecordSchema)
	return s.append(filepath.Join("memory/meta/runs", run.ID+".jsonl"), run)
}

func (s *JSONLStore) AppendEvidence(_ context.Context, ev EvidenceRef) error {
	return s.append(filepath.Join("memory/meta/evidence", ev.RunID, "evidence.jsonl"), struct {
		SchemaVersion string `json:"schema_version"`
		EvidenceRef
	}{SchemaVersion: RecordSchema, EvidenceRef: ev})
}

func (s *JSONLStore) append(rel string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateWorkspacePath(s.workspace, rel); err != nil {
		return err
	}
	path := filepath.Join(s.workspace, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

type FakeStore struct {
	Lineage  []LineageRecord
	Graph    []GraphEdge
	Runs     []ExperimentRun
	Evidence []EvidenceRef
}

func (s *FakeStore) AppendLineage(_ context.Context, rec LineageRecord) error {
	rec.SchemaVersion = nonempty(rec.SchemaVersion, RecordSchema)
	if rec.ID == "" {
		rec.ID = stableRecordID("lineage", rec.Event, rec.RunID, rec.LessonID, rec.Timestamp)
	}
	s.Lineage = append(s.Lineage, rec)
	return nil
}

func (s *FakeStore) AppendGraph(_ context.Context, edge GraphEdge) error {
	edge.SchemaVersion = nonempty(edge.SchemaVersion, RecordSchema)
	if edge.ID == "" {
		edge.ID = stableRecordID("graph", edge.Relation, edge.From, edge.To, edge.Timestamp)
	}
	s.Graph = append(s.Graph, edge)
	return nil
}

func (s *FakeStore) AppendRun(_ context.Context, run ExperimentRun) error {
	run.SchemaVersion = nonempty(run.SchemaVersion, RecordSchema)
	s.Runs = append(s.Runs, run)
	return nil
}

func (s *FakeStore) AppendEvidence(_ context.Context, ev EvidenceRef) error {
	s.Evidence = append(s.Evidence, ev)
	return nil
}

func AssertJSONLLines(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var line int
	for sc.Scan() {
		line++
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return fmt.Errorf("line %d: invalid JSON: %w", line, err)
		}
		if rec["schema_version"] == "" {
			return fmt.Errorf("line %d: missing schema_version", line)
		}
		if rec["id"] == "" {
			return fmt.Errorf("line %d: missing id", line)
		}
	}
	return sc.Err()
}

func stableRecordID(parts ...any) string {
	var out string
	for _, p := range parts {
		switch v := p.(type) {
		case time.Time:
			out += "-" + v.UTC().Format("20060102T150405Z")
		case string:
			out += "-" + slug(v)
		}
	}
	if out == "" || out == "-" {
		return "record"
	}
	return out[1:]
}

func nonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
