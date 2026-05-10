//go:build !malgo

package localaudio

import (
	"context"
	"fmt"
)

type MalgoDriver struct{}

func NewMalgoDriver() *MalgoDriver { return &MalgoDriver{} }

func (d *MalgoDriver) Status(context.Context, Config) (Status, error) {
	return Status{Enabled: false, Message: "malgo support not built; rebuild with -tags malgo"}, fmt.Errorf("malgo support not built")
}

func (d *MalgoDriver) NewCaptureStream(context.Context, StreamConfig) (CaptureStream, error) {
	return nil, fmt.Errorf("live local audio requires rebuild with -tags malgo")
}

func (d *MalgoDriver) NewPlaybackStream(context.Context, StreamConfig) (PlaybackStream, error) {
	return nil, fmt.Errorf("live local audio requires rebuild with -tags malgo")
}
