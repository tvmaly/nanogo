//go:build malgo

package localaudio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

type MalgoDriver struct{}

func NewMalgoDriver() *MalgoDriver { return &MalgoDriver{} }

func (d *MalgoDriver) Status(ctx context.Context, cfg Config) (Status, error) {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 24000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 1
	}
	audioCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return Status{Enabled: true, Message: err.Error()}, fmt.Errorf("malgo init: %w", err)
	}
	defer audioCtx.Free()

	capture, capErr := audioCtx.Devices(malgo.Capture)
	playback, playErr := audioCtx.Devices(malgo.Playback)
	if capErr != nil {
		return Status{Enabled: true, Message: capErr.Error()}, fmt.Errorf("malgo capture devices: %w", capErr)
	}
	if playErr != nil {
		return Status{Enabled: true, Message: playErr.Error()}, fmt.Errorf("malgo playback devices: %w", playErr)
	}
	select {
	case <-ctx.Done():
		return Status{Enabled: true}, ctx.Err()
	default:
	}
	return Status{
		Enabled:         true,
		CaptureDevices:  len(capture),
		PlaybackDevices: len(playback),
		Message:         "malgo audio devices available",
	}, nil
}

func (d *MalgoDriver) NewCaptureStream(ctx context.Context, cfg StreamConfig) (CaptureStream, error) {
	cfg = normalizeStreamConfig(cfg)
	audioCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = uint32(cfg.Channels)
	deviceConfig.SampleRate = uint32(cfg.SampleRate)
	deviceConfig.Alsa.NoMMap = 1

	stream := &malgoCaptureStream{
		ctx:      audioCtx,
		ch:       make(chan []byte, 32),
		done:     make(chan struct{}),
		channels: uint32(cfg.Channels),
	}
	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			stream.push(input)
		},
	}
	device, err := malgo.InitDevice(audioCtx.Context, deviceConfig, callbacks)
	if err != nil {
		_ = audioCtx.Uninit()
		audioCtx.Free()
		return nil, fmt.Errorf("malgo capture device: %w", err)
	}
	stream.device = device
	if err := device.Start(); err != nil {
		device.Uninit()
		_ = audioCtx.Uninit()
		audioCtx.Free()
		return nil, fmt.Errorf("malgo capture start: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = stream.Close()
	}()
	return stream, nil
}

func (d *MalgoDriver) NewPlaybackStream(ctx context.Context, cfg StreamConfig) (PlaybackStream, error) {
	cfg = normalizeStreamConfig(cfg)
	audioCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo init: %w", err)
	}
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(cfg.Channels)
	deviceConfig.SampleRate = uint32(cfg.SampleRate)
	deviceConfig.Alsa.NoMMap = 1

	stream := &malgoPlaybackStream{
		ctx:  audioCtx,
		cond: sync.NewCond(&sync.Mutex{}),
	}
	callbacks := malgo.DeviceCallbacks{
		Data: func(output, _ []byte, _ uint32) {
			stream.fill(output)
		},
	}
	device, err := malgo.InitDevice(audioCtx.Context, deviceConfig, callbacks)
	if err != nil {
		_ = audioCtx.Uninit()
		audioCtx.Free()
		return nil, fmt.Errorf("malgo playback device: %w", err)
	}
	stream.device = device
	if err := device.Start(); err != nil {
		device.Uninit()
		_ = audioCtx.Uninit()
		audioCtx.Free()
		return nil, fmt.Errorf("malgo playback start: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = stream.Close()
	}()
	return stream, nil
}

type malgoCaptureStream struct {
	mu       sync.Mutex
	ctx      *malgo.AllocatedContext
	device   *malgo.Device
	ch       chan []byte
	done     chan struct{}
	closed   bool
	channels uint32
}

func (s *malgoCaptureStream) Chunks() <-chan []byte { return s.ch }

func (s *malgoCaptureStream) push(input []byte) {
	if len(input) == 0 {
		return
	}
	chunk := append([]byte(nil), input...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- chunk:
	default:
	}
}

func (s *malgoCaptureStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	device := s.device
	audioCtx := s.ctx
	close(s.done)
	close(s.ch)
	s.mu.Unlock()
	if device != nil {
		device.Uninit()
	}
	if audioCtx != nil {
		_ = audioCtx.Uninit()
		audioCtx.Free()
	}
	return nil
}

type malgoPlaybackStream struct {
	cond   *sync.Cond
	ctx    *malgo.AllocatedContext
	device *malgo.Device
	buf    []byte
	closed bool
}

func (s *malgoPlaybackStream) WritePCM(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	if s.closed {
		return fmt.Errorf("playback stream closed")
	}
	s.buf = append(s.buf, pcm...)
	s.cond.Broadcast()
	return nil
}

func (s *malgoPlaybackStream) Drain(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.cond.L.Lock()
		empty := len(s.buf) == 0
		s.cond.L.Unlock()
		if empty {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *malgoPlaybackStream) Close() error {
	s.cond.L.Lock()
	if s.closed {
		s.cond.L.Unlock()
		return nil
	}
	s.closed = true
	device := s.device
	audioCtx := s.ctx
	s.cond.Broadcast()
	s.cond.L.Unlock()
	if device != nil {
		device.Uninit()
	}
	if audioCtx != nil {
		_ = audioCtx.Uninit()
		audioCtx.Free()
	}
	return nil
}

func (s *malgoPlaybackStream) fill(output []byte) {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	n := copy(output, s.buf)
	if n > 0 {
		s.buf = s.buf[n:]
	}
	for i := n; i < len(output); i++ {
		output[i] = 0
	}
	s.cond.Broadcast()
}
