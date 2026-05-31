package session

import (
	"context"
	"time"

	"github.com/tvmaly/nanogo/core/llm"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusWaiting Status = "waiting_for_input"
	StatusFailed  Status = "failed"
)

type Session interface {
	ID() string
	Messages() []llm.Message
	Append(msg llm.Message)
	Save() error
	SetWaiting(turnID string) <-chan string
	Resume(turnID, answer string)
	GetStatus() Status
}

type Store interface {
	Create(id string) (Session, error)
	Load(id string) (Session, error)
	Delete(id string) error
	GC(ctx context.Context, ttl time.Duration)
}
