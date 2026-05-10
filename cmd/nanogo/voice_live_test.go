package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tvmaly/nanogo/ext/voice/localaudio"
	"github.com/tvmaly/nanogo/ext/voice/realtime"
)

type fakeLiveSession struct {
	mu       sync.Mutex
	appended []string
	events   chan realtime.Event
	closed   bool
}

func newFakeLiveSession(events ...realtime.Event) *fakeLiveSession {
	ch := make(chan realtime.Event, len(events))
	for _, event := range events {
		ch <- event
	}
	return &fakeLiveSession{events: ch}
}

func (s *fakeLiveSession) AudioAppend(_ context.Context, audioBase64 string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appended = append(s.appended, audioBase64)
	return nil
}

func (s *fakeLiveSession) Events() <-chan realtime.Event { return s.events }

func (s *fakeLiveSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeLiveSession) Appended() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.appended...)
}

func (s *fakeLiveSession) AppendedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.appended)
}

func (s *fakeLiveSession) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func TestVoiceLiveCaptureToSession(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1, 2}, []byte{3})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	session := newFakeLiveSession()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for session.AppendedCount() < 2 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	err := runVoiceLiveLoop(ctx, capture, playback, session, voiceLiveOptions{})
	if err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	appended := session.Appended()
	if len(appended) != 2 {
		t.Fatalf("appended = %#v", appended)
	}
	first, _ := base64.StdEncoding.DecodeString(appended[0])
	second, _ := base64.StdEncoding.DecodeString(appended[1])
	if string(first) != string([]byte{1, 2}) || string(second) != string([]byte{3}) {
		t.Fatalf("decoded appends = %#v %#v", first, second)
	}
}

func TestVoiceLivePlaybackFromDeltas(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	session := newFakeLiveSession(
		realtime.Event{Type: realtime.EventResponseAudioDelta, AudioBase64: base64.StdEncoding.EncodeToString([]byte{1, 2})},
		realtime.Event{Type: realtime.EventResponseAudioDelta, AudioBase64: base64.StdEncoding.EncodeToString([]byte{3})},
		realtime.Event{Type: realtime.EventResponseAudioDone},
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for playback.DrainCount() < 1 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	err := runVoiceLiveLoop(ctx, capture, playback, session, voiceLiveOptions{})
	if err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	writes := playback.Writes()
	if len(writes) != 2 || string(writes[0]) != string([]byte{1, 2}) || string(writes[1]) != string([]byte{3}) {
		t.Fatalf("writes = %#v", writes)
	}
}

func TestVoiceLiveCancelClosesResources(t *testing.T) {
	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	session := newFakeLiveSession()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runVoiceLiveLoop(ctx, capture, playback, session, voiceLiveOptions{})
	if err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	if !capture.Closed() || !playback.Closed() || !session.Closed() {
		t.Fatalf("closed capture=%v playback=%v session=%v", capture.Closed(), playback.Closed(), session.Closed())
	}
}

func TestVoiceLiveDebugPCMOptIn(t *testing.T) {
	dir := t.TempDir()
	capturePath := filepath.Join(dir, "capture.pcm")
	playbackPath := filepath.Join(dir, "playback.pcm")

	capture := localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1})
	playback := localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	session := newFakeLiveSession(realtime.Event{Type: realtime.EventResponseAudioDelta, AudioBase64: base64.StdEncoding.EncodeToString([]byte{2})})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for session.AppendedCount() < 1 || playback.TotalBytes() < 1 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	_ = runVoiceLiveLoop(ctx, capture, playback, session, voiceLiveOptions{})
	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatalf("capture path exists without opt in: %v", err)
	}
	if _, err := os.Stat(playbackPath); !os.IsNotExist(err) {
		t.Fatalf("playback path exists without opt in: %v", err)
	}

	capture = localaudio.NewFakeCaptureStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1}, []byte{3})
	playback = localaudio.NewFakePlaybackStream(localaudio.StreamConfig{SampleRate: 24000, Channels: 1})
	session = newFakeLiveSession(realtime.Event{Type: realtime.EventResponseAudioDelta, AudioBase64: base64.StdEncoding.EncodeToString([]byte{4})})
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		for session.AppendedCount() < 1 || playback.TotalBytes() < 1 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	_ = runVoiceLiveLoop(ctx, capture, playback, session, voiceLiveOptions{
		SaveCapturePCM:  capturePath,
		SavePlaybackPCM: playbackPath,
	})
	if b, err := os.ReadFile(capturePath); err != nil || string(b) != string([]byte{3}) {
		t.Fatalf("capture debug = %v %#v", err, b)
	}
	if b, err := os.ReadFile(playbackPath); err != nil || string(b) != string([]byte{4}) {
		t.Fatalf("playback debug = %v %#v", err, b)
	}
}
