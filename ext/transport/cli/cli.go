// Package cli implements a single-shot CLI transport: reads -p flag, submits,
// waits for TurnCompleted, writes response to an io.Writer.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport"
)

func init() {
	transport.Register("cli", func(cfg json.RawMessage, bus event.Bus, app transport.App) (transport.Transport, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return New(c, bus, os.Stdout), nil
	})
}

// Config holds CLI transport configuration.
type Config struct {
	Prompt  string
	Session string
}

// Transport is the CLI single-shot transport.
type Transport struct {
	cfg Config
	bus event.Bus
	out io.Writer
}

// New constructs a CLI transport.
func New(cfg Config, bus event.Bus, out io.Writer) *Transport {
	if cfg.Session == "" {
		cfg.Session = "cli"
	}
	return &Transport{cfg: cfg, bus: bus, out: out}
}

func (t *Transport) Name() string { return "cli" }

// Start submits the prompt, waits for TurnCompleted or Error, writes output.
func (t *Transport) Start(ctx context.Context, app transport.App) error {
	doneCh := t.bus.Subscribe(ctx, event.TurnCompleted, event.Error)

	if err := app.Submit(ctx, t.cfg.Session, t.cfg.Prompt); err != nil {
		return err
	}

	for {
		select {
		case e, ok := <-doneCh:
			if !ok {
				return ctx.Err()
			}
			if e.Session != "" && e.Session != t.cfg.Session {
				continue
			}
			switch e.Kind {
			case event.TurnCompleted:
				if p, ok := e.Payload.(event.TurnCompletedPayload); ok {
					fmt.Fprint(t.out, p.Text)
				}
				return nil
			case event.Error:
				if msg, ok := e.Payload.(string); ok {
					return errors.New(msg)
				}
				return errors.New("agent error")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (t *Transport) Stop(_ context.Context) error { return nil }
