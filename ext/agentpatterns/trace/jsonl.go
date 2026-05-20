package trace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/tvmaly/nanogo/core/contracts"
)

type Config struct {
	Redact bool
}

type JSONLSink struct {
	path string
	cfg  Config
}

var _ contracts.TraceSink = (*JSONLSink)(nil)

func NewJSONLSink(path string, cfg Config) *JSONLSink {
	return &JSONLSink{path: path, cfg: cfg}
}

func (s *JSONLSink) EmitTrace(_ context.Context, event contracts.TraceEvent) error {
	if event.Version == 0 {
		event.Version = 1
	}
	if s.cfg.Redact {
		event.SessionID = redactValue(event.SessionID)
		event.Data = redactMap(event.Data)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(map[string]any{
		"version":    event.Version,
		"run_id":     event.RunID,
		"session_id": event.SessionID,
		"pattern":    event.Pattern,
		"node":       event.Node,
		"step":       event.Step,
		"kind":       event.Kind,
		"status":     event.Status,
		"message":    event.Message,
		"data":       event.Data,
		"created_at": event.CreatedAt,
	})
}

func redactMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "child") {
			out[k] = "[redacted]"
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = redactValue(s)
		} else {
			out[k] = v
		}
	}
	return out
}

func redactValue(s string) string {
	if s == "" {
		return s
	}
	return "[redacted]"
}
