// Package repl implements an interactive read-eval-print-loop transport.
package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport"
)

func init() {
	transport.Register("repl", func(cfg json.RawMessage, bus event.Bus, app transport.App) (transport.Transport, error) {
		var c Config
		if len(cfg) > 0 {
			if err := json.Unmarshal(cfg, &c); err != nil {
				return nil, err
			}
		}
		return New(c, bus, app, os.Stdin, os.Stdout), nil
	})
}

// Config holds REPL transport configuration.
type Config struct {
	Workspace string `json:"workspace"`
}

// Transport is the interactive REPL transport.
type Transport struct {
	cfg     Config
	bus     event.Bus
	app     transport.App
	in      io.Reader
	out     io.Writer
	session string
}

// New constructs a REPL Transport with injectable I/O (useful for tests).
func New(cfg Config, bus event.Bus, app transport.App, in io.Reader, out io.Writer) *Transport {
	return &Transport{
		cfg:     cfg,
		bus:     bus,
		app:     app,
		in:      in,
		out:     out,
		session: newID(),
	}
}

func (t *Transport) Name() string { return "repl" }

// Start runs the interactive loop. Returns when EOF or /exit is received.
func (t *Transport) Start(ctx context.Context, app transport.App) error {
	if app != nil {
		t.app = app
	}
	sc := bufio.NewScanner(t.in)
	fmt.Fprint(t.out, "> ")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			fmt.Fprint(t.out, "> ")
			continue
		}
		if strings.HasPrefix(line, "/") {
			if done := t.handleSlash(ctx, line); done {
				return nil
			}
			fmt.Fprint(t.out, "> ")
			continue
		}
		t.runTurn(ctx, line)
		fmt.Fprint(t.out, "> ")
	}
	fmt.Fprintln(t.out)
	return nil
}

func (t *Transport) Stop(_ context.Context) error { return nil }

func (t *Transport) handleSlash(ctx context.Context, cmd string) (exit bool) {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/exit", "/quit":
		return true
	case "/new":
		t.session = newID()
		fmt.Fprintln(t.out, "[new session]")
	case "/help":
		fmt.Fprintln(t.out, "Commands: /help  /new  /memories  /dream  /exit")
	case "/memories":
		t.showMemories()
	case "/dream":
		fmt.Fprintln(t.out, "[dream] Run 'nanogo dream' to trigger a consolidation cycle.")
	default:
		fmt.Fprintf(t.out, "unknown command %q — type /help for help\n", parts[0])
	}
	return false
}

func (t *Transport) showMemories() {
	workspace := t.cfg.Workspace
	if workspace == "" {
		home, _ := os.UserHomeDir()
		workspace = home + "/.nanogo/workspace"
	}
	data, err := os.ReadFile(workspace + "/MEMORY.md")
	if err != nil {
		fmt.Fprintln(t.out, "[memories] no memory file found")
		return
	}
	fmt.Fprintln(t.out, string(data))
}

func (t *Transport) runTurn(ctx context.Context, msg string) {
	evtCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sub := t.bus.Subscribe(evtCtx, event.TokenDelta, event.TurnCompleted, event.Error)

	if err := t.app.Submit(ctx, t.session, msg); err != nil {
		fmt.Fprintf(t.out, "error: %v\n", err)
		return
	}

	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return
			}
			if e.Session != "" && e.Session != t.session {
				continue
			}
			switch e.Kind {
			case event.TokenDelta:
				if s, ok := e.Payload.(string); ok {
					fmt.Fprint(t.out, s)
				}
			case event.TurnCompleted:
				fmt.Fprintln(t.out)
				return
			case event.Error:
				fmt.Fprintf(t.out, "\nerror: %v\n", e.Payload)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

var idCounter atomic.Int64

func newID() string {
	n := idCounter.Add(1)
	return fmt.Sprintf("repl-%d-%d", n, time.Now().UnixNano())
}
