package localaudio

import (
	"context"
	"fmt"
	"sync"
)

type Config struct {
	SampleRate int
	Channels   int
}

type StreamConfig struct {
	SampleRate int
	Channels   int
}

type Status struct {
	Enabled         bool
	CaptureDevices  int
	PlaybackDevices int
	Message         string
}

type Driver interface {
	Status(ctx context.Context, cfg Config) (Status, error)
	NewCaptureStream(ctx context.Context, cfg StreamConfig) (CaptureStream, error)
	NewPlaybackStream(ctx context.Context, cfg StreamConfig) (PlaybackStream, error)
}

type CaptureStream interface {
	Chunks() <-chan []byte
	Close() error
}

type PlaybackStream interface {
	WritePCM(ctx context.Context, pcm []byte) error
	Drain(ctx context.Context) error
	Close() error
}

func DefaultConfig() Config {
	return Config{SampleRate: 24000, Channels: 1}
}

func DefaultStreamConfig() StreamConfig {
	return StreamConfig{SampleRate: 24000, Channels: 1}
}

func normalizeStreamConfig(cfg StreamConfig) StreamConfig {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 24000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 1
	}
	return cfg
}

type FakeCaptureStream struct {
	Config StreamConfig

	mu     sync.Mutex
	ch     chan []byte
	closed bool
}

func NewFakeCaptureStream(cfg StreamConfig, chunks ...[]byte) *FakeCaptureStream {
	s := &FakeCaptureStream{
		Config: normalizeStreamConfig(cfg),
		ch:     make(chan []byte, len(chunks)),
	}
	for _, chunk := range chunks {
		cp := append([]byte(nil), chunk...)
		s.ch <- cp
	}
	close(s.ch)
	return s
}

func (s *FakeCaptureStream) Chunks() <-chan []byte {
	return s.ch
}

func (s *FakeCaptureStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *FakeCaptureStream) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

type FakePlaybackStream struct {
	Config StreamConfig

	mu     sync.Mutex
	writes [][]byte
	total  int
	drains int
	closed bool
}

func NewFakePlaybackStream(cfg StreamConfig) *FakePlaybackStream {
	return &FakePlaybackStream{Config: normalizeStreamConfig(cfg)}
}

func (s *FakePlaybackStream) WritePCM(ctx context.Context, pcm []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("playback stream closed")
	}
	cp := append([]byte(nil), pcm...)
	s.writes = append(s.writes, cp)
	s.total += len(cp)
	return nil
}

func (s *FakePlaybackStream) Drain(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drains++
	return nil
}

func (s *FakePlaybackStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *FakePlaybackStream) Writes() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.writes))
	for i := range s.writes {
		out[i] = append([]byte(nil), s.writes[i]...)
	}
	return out
}

func (s *FakePlaybackStream) TotalBytes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

func (s *FakePlaybackStream) DrainCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drains
}

func (s *FakePlaybackStream) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
