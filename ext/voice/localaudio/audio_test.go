package localaudio

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalAudioDriverStatusIsIsolated(t *testing.T) {
	driver := NewMalgoDriver()
	status, err := driver.Status(context.Background(), DefaultConfig())
	if err != nil && status.Message == "" {
		t.Fatalf("missing skip/status message for error: %v", err)
	}
}

func TestLocalAudioStreamingInterfaces(t *testing.T) {
	var _ CaptureStream = (*FakeCaptureStream)(nil)
	var _ PlaybackStream = (*FakePlaybackStream)(nil)
	driver := NewMalgoDriver()
	if driver == nil {
		t.Fatal("missing local audio driver")
	}
}

func TestFakeCaptureStreamChunks(t *testing.T) {
	stream := NewFakeCaptureStream(StreamConfig{SampleRate: 24000, Channels: 1}, []byte{1, 2}, []byte{3, 4})
	if stream.Config.SampleRate != 24000 || stream.Config.Channels != 1 {
		t.Fatalf("config = %#v", stream.Config)
	}

	var got [][]byte
	for chunk := range stream.Chunks() {
		got = append(got, chunk)
	}
	if len(got) != 2 || !bytes.Equal(got[0], []byte{1, 2}) || !bytes.Equal(got[1], []byte{3, 4}) {
		t.Fatalf("chunks = %#v", got)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFakePlaybackStreamDrain(t *testing.T) {
	stream := NewFakePlaybackStream(StreamConfig{SampleRate: 24000, Channels: 1})
	if err := stream.WritePCM(context.Background(), []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := stream.WritePCM(context.Background(), []byte{3}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stream.TotalBytes() != 3 {
		t.Fatalf("total bytes = %d", stream.TotalBytes())
	}
	writes := stream.Writes()
	if len(writes) != 2 || !bytes.Equal(writes[0], []byte{1, 2}) || !bytes.Equal(writes[1], []byte{3}) {
		t.Fatalf("writes = %#v", writes)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}
