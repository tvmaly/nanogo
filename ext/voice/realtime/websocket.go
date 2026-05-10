package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coder/websocket"
)

// WebSocketConn wraps coder/websocket with normalized Event JSON handling.
type WebSocketConn struct {
	conn *websocket.Conn
}

// DialWebSocket opens a text-message websocket connection.
func DialWebSocket(ctx context.Context, url string, headers map[string]string) (*WebSocketConn, error) {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			msg := strings.TrimSpace(string(body))
			if msg != "" {
				return nil, fmt.Errorf("websocket dial: HTTP %d: %s: %w", resp.StatusCode, msg, err)
			}
			return nil, fmt.Errorf("websocket dial: HTTP %d: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	conn.SetReadLimit(16 << 20)
	return &WebSocketConn{conn: conn}, nil
}

func (c *WebSocketConn) Send(ctx context.Context, event Event) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return fmt.Errorf("websocket send: %w", err)
	}
	return nil
}

func (c *WebSocketConn) Receive(ctx context.Context) (Event, error) {
	typ, b, err := c.conn.Read(ctx)
	if err != nil {
		if status := websocket.CloseStatus(err); status != -1 {
			return Event{}, fmt.Errorf("websocket closed: %v", status)
		}
		return Event{}, fmt.Errorf("websocket receive: %w", err)
	}
	if typ != websocket.MessageText {
		return Event{}, fmt.Errorf("websocket receive: unexpected message type %v", typ)
	}
	var event Event
	if err := json.Unmarshal(b, &event); err != nil {
		return Event{}, fmt.Errorf("websocket receive: decode event: %w", err)
	}
	if len(event.Raw) == 0 {
		event.Raw = append([]byte(nil), b...)
	}
	return event, nil
}

func (c *WebSocketConn) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "voice session closed")
}
