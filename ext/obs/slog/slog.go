// Package slog adapts log/slog to core obs.
package slog

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/tvmaly/nanogo/core/obs"
)

type Config struct {
	Format string `json:"format"`
	Level  string `json:"level"`
}

type Logger struct{ l *slog.Logger }

func New(cfg Config, w io.Writer) *Logger {
	if w == nil {
		w = os.Stderr
	}
	var h slog.Handler
	if cfg.Format == "json" {
		h = slog.NewJSONHandler(w, nil)
	} else {
		h = slog.NewTextHandler(w, nil)
	}
	return &Logger{l: slog.New(h)}
}

func (l *Logger) Log(ctx context.Context, e obs.Entry) error {
	args := make([]any, 0, len(e.Attrs)*2)
	for k, v := range e.Attrs {
		args = append(args, k, v)
	}
	l.l.Log(ctx, slog.LevelInfo, e.Message, args...)
	return nil
}
