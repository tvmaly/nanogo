// Package ffmpeg defines a bounded frame sampling contract and command builder.
package ffmpeg

import (
	"context"
	"fmt"
	"path/filepath"
)

type Sampler struct {
	Binary string
}

type Request struct {
	Input     string
	OutputDir string
	FPS       int
	MaxFrames int
	Height    int
}

type Frame struct {
	Path  string
	Index int
}

func (s Sampler) Command(req Request) ([]string, error) {
	if req.Input == "" || req.OutputDir == "" {
		return nil, fmt.Errorf("input and output_dir are required")
	}
	if req.FPS <= 0 {
		req.FPS = 8
	}
	if req.MaxFrames <= 0 {
		req.MaxFrames = 24
	}
	if req.Height <= 0 {
		req.Height = 720
	}
	bin := s.Binary
	if bin == "" {
		bin = "ffmpeg"
	}
	return []string{bin, "-i", req.Input, "-vf", fmt.Sprintf("fps=%d,scale=-2:%d", req.FPS, req.Height), "-frames:v", fmt.Sprintf("%d", req.MaxFrames), filepath.Join(req.OutputDir, "frame-%03d.jpg")}, nil
}

func (s Sampler) Sample(ctx context.Context, req Request, run func(context.Context, []string) error) ([]Frame, error) {
	cmd, err := s.Command(req)
	if err != nil {
		return nil, err
	}
	if run != nil {
		if err := run(ctx, cmd); err != nil {
			return nil, err
		}
	}
	frames := make([]Frame, req.MaxFrames)
	if req.MaxFrames <= 0 {
		frames = make([]Frame, 24)
	}
	for i := range frames {
		frames[i] = Frame{Path: filepath.Join(req.OutputDir, fmt.Sprintf("frame-%03d.jpg", i+1)), Index: i + 1}
	}
	return frames, nil
}
