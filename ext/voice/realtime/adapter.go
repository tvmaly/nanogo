package realtime

import "context"

// ProviderConfig contains provider-agnostic connection settings.
type ProviderConfig struct {
	URL          string
	APIKey       string
	Model        string
	Provider     string
	Instructions string
	Voice        string
	Headers      map[string]string
}

// ProviderAdapter opens realtime provider sessions.
type ProviderAdapter interface {
	Name() string
	Connect(ctx context.Context, cfg ProviderConfig) (RealtimeConn, error)
}

// RealtimeConn sends and receives normalized events.
type RealtimeConn interface {
	Send(ctx context.Context, event Event) error
	Receive(ctx context.Context) (Event, error)
	Close() error
}
