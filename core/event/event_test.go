package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/core/event"
)

func TestBus_PubSubRoundtrip(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := bus.Subscribe(ctx, event.TurnStarted)
	e := event.Event{Kind: event.TurnStarted, Session: "s1", Turn: 1, At: time.Now()}
	bus.Publish(e)

	select {
	case got := <-ch:
		if got.Kind != event.TurnStarted {
			t.Fatalf("expected TurnStarted, got %s", got.Kind)
		}
		if got.Session != "s1" {
			t.Fatalf("expected session s1, got %s", got.Session)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_NoEventsBeforeSubscribe(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Publish before subscribe
	bus.Publish(event.Event{Kind: event.TurnStarted, Session: "early"})

	ch := bus.Subscribe(ctx, event.TurnStarted)

	// Publish after subscribe
	bus.Publish(event.Event{Kind: event.TurnStarted, Session: "after"})

	select {
	case got := <-ch:
		if got.Session == "early" {
			t.Fatal("received event that was published before subscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-subscribe event")
	}
}

func TestBusBackpressure(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := bus.Subscribe(ctx, event.TurnStarted)

	// Publish 257 events to a non-reading subscriber
	for i := 0; i < 257; i++ {
		bus.Publish(event.Event{Kind: event.TurnStarted, Turn: i})
	}

	// Drain all available events
	received := 0
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-ch:
			received++
		case <-deadline:
			goto done
		}
	}
done:
	if received != 256 {
		t.Fatalf("expected 256 events (drop-oldest), got %d", received)
	}
}

func TestBusUnsubscribe(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())

	ch := bus.Subscribe(ctx, event.TurnStarted)
	cancel() // unsubscribe

	// Give bus time to process cancellation
	time.Sleep(50 * time.Millisecond)

	bus.Publish(event.Event{Kind: event.TurnStarted})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := bus.Subscribe(ctx, event.TurnStarted)
	ch2 := bus.Subscribe(ctx, event.TurnStarted)

	bus.Publish(event.Event{Kind: event.TurnStarted, Session: "multi"})

	for _, ch := range []<-chan event.Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Session != "multi" {
				t.Fatalf("wrong session: %s", got.Session)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive event")
		}
	}
}

func TestBus_KindFiltering(t *testing.T) {
	t.Parallel()
	bus := event.NewBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := bus.Subscribe(ctx, event.TurnCompleted) // only TurnCompleted

	bus.Publish(event.Event{Kind: event.TurnStarted, Session: "wrong"})
	bus.Publish(event.Event{Kind: event.TurnCompleted, Session: "right"})

	select {
	case got := <-ch:
		if got.Session != "right" {
			t.Fatalf("expected 'right', got %s", got.Session)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
