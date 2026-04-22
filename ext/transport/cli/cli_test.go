package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	eventfake "github.com/tvmaly/nanogo/core/event/fake"
	"github.com/tvmaly/nanogo/ext/transport/cli"
)

// fakeApp implements transport.App for tests.
type fakeApp struct {
	submitted []string
	bus       *eventfake.Bus
}

func (f *fakeApp) Submit(ctx context.Context, session, message string) error {
	f.submitted = append(f.submitted, message)
	// simulate agent completing the turn
	go func() {
		time.Sleep(10 * time.Millisecond)
		f.bus.Publish(event.Event{
			Kind:    event.TurnCompleted,
			Session: session,
			Payload: event.TurnCompletedPayload{Text: "4"},
		})
	}()
	return nil
}

func (f *fakeApp) Resume(_ context.Context, _, _ string) error { return nil }
func (f *fakeApp) TriggerSkill(_ context.Context, _ string, _ map[string]any) error {
	return nil
}

func TestCLI_SingleShot(t *testing.T) {
	t.Parallel()

	bus := eventfake.New()
	app := &fakeApp{bus: bus}
	var out bytes.Buffer

	tr := cli.New(cli.Config{Prompt: "2+2?"}, bus, &out)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := tr.Start(ctx, app); err != nil {
		t.Fatal(err)
	}

	if len(app.submitted) != 1 || app.submitted[0] != "2+2?" {
		t.Fatalf("unexpected submitted: %v", app.submitted)
	}
	if out.String() != "4" {
		t.Fatalf("expected output '4', got %q", out.String())
	}
}

func TestCLI_ErrorEvent(t *testing.T) {
	t.Parallel()

	bus := eventfake.New()
	app2 := &errorApp{bus: bus}
	var out bytes.Buffer

	tr := cli.New(cli.Config{Prompt: "bad?"}, bus, &out)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := tr.Start(ctx, app2)
	if err == nil {
		t.Fatal("expected error from CLI transport on Error event")
	}
}

type errorApp struct {
	bus *eventfake.Bus
}

func (e *errorApp) Submit(ctx context.Context, session, _ string) error {
	go func() {
		time.Sleep(10 * time.Millisecond)
		e.bus.Publish(event.Event{
			Kind:    event.Error,
			Session: session,
			Payload: "something went wrong",
		})
	}()
	return nil
}
func (e *errorApp) Resume(_ context.Context, _, _ string) error          { return nil }
func (e *errorApp) TriggerSkill(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
