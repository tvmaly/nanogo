package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/tvmaly/nanogo/ext/voice/localaudio"
	"github.com/tvmaly/nanogo/ext/voice/providers/xai"
	"github.com/tvmaly/nanogo/ext/voice/realtime"
	voicesession "github.com/tvmaly/nanogo/ext/voice/session"
)

func runVoiceCmd(args []string, workspace string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo voice smoke|live --provider xai [options]")
	}
	switch args[0] {
	case "smoke":
		return runVoiceSmoke(args[1:], workspace)
	case "live":
		return runVoiceLive(args[1:], workspace)
	default:
		return fmt.Errorf("unknown voice command %q", args[0])
	}
}

func runVoiceSmoke(args []string, workspace string) error {
	fs := flag.NewFlagSet("voice smoke", flag.ContinueOnError)
	providerName := fs.String("provider", "xai", "voice provider")
	child := fs.String("child", "", "child id")
	text := fs.String("text", "", "text prompt for deterministic smoke")
	audioIn := fs.String("audio-in", "", "raw PCM16 24kHz mono input file")
	audioOut := fs.String("audio-out", "", "raw PCM16 24kHz mono output file")
	mic := fs.Bool("mic", false, "evaluate local microphone capture with malgo")
	speaker := fs.Bool("speaker", false, "evaluate local speaker playback with malgo")
	timeout := fs.Duration("timeout", 20*time.Second, "smoke-test timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *providerName != "xai" {
		return fmt.Errorf("unsupported voice provider %q", *providerName)
	}
	if *mic || *speaker {
		status, err := localaudio.NewMalgoDriver().Status(context.Background(), localaudio.DefaultConfig())
		if err != nil {
			fmt.Printf("local audio skipped: %s\n", status.Message)
			if *text == "" && *audioIn == "" {
				return nil
			}
		} else {
			fmt.Printf("local audio: capture_devices=%d playback_devices=%d\n", status.CaptureDevices, status.PlaybackDevices)
			if *text == "" && *audioIn == "" {
				return nil
			}
		}
	}

	cfg, err := xai.ConfigFromEnv()
	if err != nil {
		return err
	}
	if *child != "" {
		cfg.Instructions = "You are a concise voice tutor helping child " + *child + "."
	}
	adapter := xai.New(cfg)
	mgr := voicesession.NewManager(voicesession.Config{
		Workspace: workspace,
		Provider:  adapter,
		ProviderCfg: realtime.ProviderConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		},
		SessionUpdate: xai.SessionUpdate(cfg),
	})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	s, err := mgr.Start(ctx)
	if err != nil {
		return err
	}
	defer mgr.Close(s.ID)
	events, err := mgr.Events(s.ID)
	if err != nil {
		return err
	}
	fmt.Printf("voice session: %s provider=xai model=%s\n", s.ID, cfg.Model)

	if *text != "" {
		if err := mgr.TextSend(ctx, s.ID, *text); err != nil {
			return err
		}
		if err := mgr.ResponseCreate(ctx, s.ID); err != nil {
			return err
		}
	} else if *audioIn != "" {
		b, err := os.ReadFile(*audioIn)
		if err != nil {
			return err
		}
		if err := mgr.AudioAppend(ctx, s.ID, base64.StdEncoding.EncodeToString(b)); err != nil {
			return err
		}
		if err := mgr.AudioCommit(ctx, s.ID); err != nil {
			return err
		}
		if err := mgr.ResponseCreate(ctx, s.ID); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("voice smoke requires --text or --audio-in unless only checking --mic/--speaker")
	}

	var out []byte
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			fmt.Printf("event: %s\n", event.Type)
			if event.Text != "" {
				fmt.Printf("text: %s\n", event.Text)
			}
			if event.AudioBase64 != "" && *audioOut != "" {
				chunk, err := base64.StdEncoding.DecodeString(event.AudioBase64)
				if err == nil {
					out = append(out, chunk...)
				}
			}
			if event.Type == realtime.EventResponseDone || event.Type == realtime.EventError {
				if *audioOut != "" && len(out) > 0 {
					if err := os.MkdirAll(filepath.Dir(*audioOut), 0755); err != nil {
						return err
					}
					if err := os.WriteFile(*audioOut, out, 0644); err != nil {
						return err
					}
				}
				if event.Type == realtime.EventError {
					return fmt.Errorf("%s", event.Error)
				}
				return nil
			}
		case <-ctx.Done():
			if *audioOut != "" && len(out) > 0 {
				_ = os.WriteFile(*audioOut, out, 0644)
			}
			return ctx.Err()
		}
	}
}

type voiceLiveOptions struct {
	SaveCapturePCM  string
	SavePlaybackPCM string
}

type voiceLiveSession interface {
	AudioAppend(ctx context.Context, audioBase64 string) error
	Events() <-chan realtime.Event
	Close() error
}

type managerLiveSession struct {
	mgr    *voicesession.Manager
	id     string
	events <-chan realtime.Event
}

func (s *managerLiveSession) AudioAppend(ctx context.Context, audioBase64 string) error {
	return s.mgr.AudioAppend(ctx, s.id, audioBase64)
}

func (s *managerLiveSession) Events() <-chan realtime.Event { return s.events }

func (s *managerLiveSession) Close() error { return s.mgr.Close(s.id) }

func runVoiceLive(args []string, workspace string) error {
	fs := flag.NewFlagSet("voice live", flag.ContinueOnError)
	providerName := fs.String("provider", "xai", "voice provider")
	child := fs.String("child", "", "child id")
	saveCapture := fs.String("save-capture-pcm", "", "debug path for raw captured PCM")
	savePlayback := fs.String("save-playback-pcm", "", "debug path for raw playback PCM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *providerName != "xai" {
		return fmt.Errorf("unsupported voice provider %q", *providerName)
	}

	cfg, err := xai.ConfigFromEnv()
	if err != nil {
		return err
	}
	if *child != "" {
		cfg.Instructions = "You are a concise voice tutor helping child " + *child + "."
	}
	adapter := xai.New(cfg)
	mgr := voicesession.NewManager(voicesession.Config{
		Workspace: workspace,
		Provider:  adapter,
		ProviderCfg: realtime.ProviderConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
			URL:    cfg.URL,
		},
		SessionUpdate: xai.SessionUpdate(cfg),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, err := mgr.Start(ctx)
	if err != nil {
		return err
	}
	events, err := mgr.Events(s.ID)
	if err != nil {
		_ = mgr.Close(s.ID)
		return err
	}

	driver := localaudio.NewMalgoDriver()
	streamCfg := localaudio.DefaultStreamConfig()
	capture, err := driver.NewCaptureStream(ctx, streamCfg)
	if err != nil {
		_ = mgr.Close(s.ID)
		return err
	}
	playback, err := driver.NewPlaybackStream(ctx, streamCfg)
	if err != nil {
		_ = capture.Close()
		_ = mgr.Close(s.ID)
		return err
	}

	fmt.Printf("voice live session: %s provider=xai model=%s sample_rate=%d channels=%d\n", s.ID, cfg.Model, streamCfg.SampleRate, streamCfg.Channels)
	fmt.Println("speak now; press Ctrl-C to stop")
	err = runVoiceLiveLoop(ctx, capture, playback, &managerLiveSession{mgr: mgr, id: s.ID, events: events}, voiceLiveOptions{
		SaveCapturePCM:  *saveCapture,
		SavePlaybackPCM: *savePlayback,
	})
	if err == context.Canceled {
		fmt.Println("voice live stopped")
		return nil
	}
	return err
}

func runVoiceLiveLoop(ctx context.Context, capture localaudio.CaptureStream, playback localaudio.PlaybackStream, session voiceLiveSession, opts voiceLiveOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer session.Close()
	defer playback.Close()
	defer capture.Close()

	captureOut, err := optionalPCMWriter(opts.SaveCapturePCM)
	if err != nil {
		return err
	}
	defer captureOut.Close()
	playbackOut, err := optionalPCMWriter(opts.SavePlaybackPCM)
	if err != nil {
		return err
	}
	defer playbackOut.Close()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-capture.Chunks():
				if !ok {
					return
				}
				if len(chunk) == 0 {
					continue
				}
				if _, err := captureOut.Write(chunk); err != nil {
					errCh <- err
					cancel()
					return
				}
				if err := session.AudioAppend(ctx, base64.StdEncoding.EncodeToString(chunk)); err != nil {
					errCh <- err
					cancel()
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-session.Events():
				if !ok {
					return
				}
				switch event.Type {
				case realtime.EventResponseAudioDelta:
					chunk, err := base64.StdEncoding.DecodeString(event.AudioBase64)
					if err != nil {
						errCh <- fmt.Errorf("decode response audio delta: %w", err)
						cancel()
						return
					}
					if _, err := playbackOut.Write(chunk); err != nil {
						errCh <- err
						cancel()
						return
					}
					if err := playback.WritePCM(ctx, chunk); err != nil {
						errCh <- err
						cancel()
						return
					}
				case realtime.EventResponseAudioDone:
					if err := playback.Drain(ctx); err != nil {
						errCh <- err
						cancel()
						return
					}
				case realtime.EventError:
					errCh <- fmt.Errorf("%s", event.Error)
					cancel()
					return
				}
			}
		}
	}()

	select {
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	case <-ctx.Done():
		wg.Wait()
		return ctx.Err()
	}
}

type pcmWriter struct {
	io.Writer
	close func() error
}

func (w pcmWriter) Close() error {
	if w.close == nil {
		return nil
	}
	return w.close()
}

func optionalPCMWriter(path string) (pcmWriter, error) {
	if path == "" {
		return pcmWriter{Writer: io.Discard}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return pcmWriter{}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return pcmWriter{}, err
	}
	return pcmWriter{Writer: f, close: f.Close}, nil
}
