package fake

import (
	"context"
	"io"
	"sync"

	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

type Adapter struct {
	Conn *Conn
}

func New(events ...realtime.Event) *Adapter {
	return &Adapter{Conn: NewConn(events...)}
}

func (a *Adapter) Name() string { return "fake" }

func (a *Adapter) Connect(context.Context, realtime.ProviderConfig) (realtime.RealtimeConn, error) {
	return a.Conn, nil
}

type Conn struct {
	mu       sync.Mutex
	Sent     []realtime.Event
	incoming []realtime.Event
	closed   bool
}

func NewConn(events ...realtime.Event) *Conn {
	return &Conn{incoming: append([]realtime.Event(nil), events...)}
}

func (c *Conn) Send(_ context.Context, event realtime.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sent = append(c.Sent, event)
	return nil
}

func (c *Conn) Receive(context.Context) (realtime.Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.incoming) == 0 {
		return realtime.Event{}, io.EOF
	}
	e := c.incoming[0]
	c.incoming = c.incoming[1:]
	return e, nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}
