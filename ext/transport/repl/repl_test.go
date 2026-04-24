package repl_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/transport/fake"
	"github.com/tvmaly/nanogo/ext/transport/repl"
)

// stubApp wraps fake.App and emits a TurnCompleted after Submit.
func makeApp(bus event.Bus) *fake.App {
	return &fake.App{Bus: bus}
}

// TEST-5.6 — REPL exits cleanly on EOF (Ctrl+D)
func TestREPLExitsOnEOF(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := makeApp(bus)
	var out bytes.Buffer
	r := repl.New(repl.Config{}, bus, app, strings.NewReader(""), &out)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Start(ctx, app); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TEST-5.6 — REPL submits a user message and prints prompt
func TestREPLSubmitsMessage(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := makeApp(bus)
	var out bytes.Buffer
	r := repl.New(repl.Config{}, bus, app, strings.NewReader("hello\n"), &out)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = r.Start(ctx, app)

	if len(app.Submits) != 1 || app.Submits[0].Message != "hello" {
		t.Fatalf("expected Submit('hello'), got %+v", app.Submits)
	}
}

// TEST-5.7 — /exit slash command exits cleanly
func TestREPLExitCommand(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := makeApp(bus)
	var out bytes.Buffer
	r := repl.New(repl.Config{}, bus, app, strings.NewReader("/exit\n"), &out)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Start(ctx, app); err != nil {
		t.Fatalf("/exit should exit cleanly: %v", err)
	}
}

// TEST-5.7 — /new clears session (no Submit before /new)
func TestREPLNewCommand(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := makeApp(bus)
	var out bytes.Buffer
	r := repl.New(repl.Config{}, bus, app, strings.NewReader("/new\n/exit\n"), &out)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = r.Start(ctx, app)

	if len(app.Submits) != 0 {
		t.Fatalf("/new should not submit, got %+v", app.Submits)
	}
	if !strings.Contains(out.String(), "new session") {
		t.Fatalf("expected 'new session' in output, got %q", out.String())
	}
}

// TEST-5.7 — /help lists commands
func TestREPLHelp(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	app := makeApp(bus)
	var out bytes.Buffer
	r := repl.New(repl.Config{}, bus, app, strings.NewReader("/help\n/exit\n"), &out)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = r.Start(ctx, app)

	if !strings.Contains(out.String(), "/help") {
		t.Fatalf("expected /help in output, got %q", out.String())
	}
}
