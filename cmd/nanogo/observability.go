package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	costobs "github.com/tvmaly/nanogo/ext/obs/cost"
	fileobs "github.com/tvmaly/nanogo/ext/obs/file"
	slogobs "github.com/tvmaly/nanogo/ext/obs/slog"
	"github.com/tvmaly/nanogo/modules/obs"
	obsjsonl "github.com/tvmaly/nanogo/modules/obs/jsonl"
)

func startObs(ctx context.Context, bus event.Bus, cfg *config) (func(), error) {
	var cancels []context.CancelFunc
	obs.Reset()
	for _, entry := range cfg.Obs {
		switch entry.Driver {
		case "slog":
			var c slogobs.Config
			_ = json.Unmarshal(entry.Config, &c)
			obs.SetLoggers(slogobs.New(c, os.Stderr))
		case "file":
			var c fileobs.Config
			if err := json.Unmarshal(entry.Config, &c); err != nil {
				return nil, err
			}
			c.Path = expandPath(c.Path)
			w, err := fileobs.New(c)
			if err != nil {
				return nil, err
			}
			subCtx, cancel := context.WithCancel(ctx)
			cancels = append(cancels, func() { cancel(); _ = w.Close() })
			go recordEvents(subCtx, bus, w.Record)
		case "cost":
			var c costobs.Config
			if err := json.Unmarshal(entry.Config, &c); err != nil {
				return nil, err
			}
			c.OutputPath = expandPath(c.OutputPath)
			t := costobs.New(c)
			subCtx, cancel := context.WithCancel(ctx)
			cancels = append(cancels, cancel)
			go recordEvents(subCtx, bus, t.Record)
		case "jsonl":
			var c obsjsonl.Config
			if err := json.Unmarshal(entry.Config, &c); err != nil {
				return nil, err
			}
			c.Root = expandPath(c.Root)
			c.Path = expandPath(c.Path)
			store, err := obsjsonl.New(c)
			if err != nil {
				return nil, err
			}
			observer := obs.NewEventObserver(store, obs.ObserverConfig{
				FailurePolicy:    c.FailurePolicy,
				FlushOnError:     c.FlushOnError,
				FlushOnRunFinish: c.FlushOnRunFinish,
			})
			subCtx, cancel := context.WithCancel(ctx)
			cancels = append(cancels, func() { cancel(); _ = store.Close() })
			go recordEvents(subCtx, bus, observer.Observe)
		default:
			return nil, fmt.Errorf("obs driver: unknown driver %q", entry.Driver)
		}
	}
	return func() {
		time.Sleep(100 * time.Millisecond)
		for _, c := range cancels {
			c()
		}
	}, nil
}
