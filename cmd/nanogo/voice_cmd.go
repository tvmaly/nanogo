package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tvmaly/nanogo/core/contracts"
	contractfake "github.com/tvmaly/nanogo/core/contracts/fake"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/ext/voice/localaudio"
	appleavspeech "github.com/tvmaly/nanogo/ext/voice/providers/apple/avspeech"
	applespeechanalyzer "github.com/tvmaly/nanogo/ext/voice/providers/apple/speechanalyzer"
	"github.com/tvmaly/nanogo/ext/voice/providers/xai"
	"github.com/tvmaly/nanogo/ext/voice/realtime"
	voicesession "github.com/tvmaly/nanogo/ext/voice/session"
	voice "github.com/tvmaly/nanogo/modules/voice"
)

func runVoiceCmd(args []string, workspace string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo voice smoke|live --provider xai [options]")
	}
	switch args[0] {
	case "providers":
		return runVoiceProviders(context.Background(), os.Stdout, defaultVoiceRegistry())
	case "stt":
		return runVoiceSTTCmd(args[1:])
	case "tts":
		return runVoiceTTSCmd(args[1:])
	case "chat":
		return runVoiceChatCmd(args[1:])
	case "smoke":
		return runVoiceSmoke(args[1:], workspace)
	case "live":
		return runVoiceLive(args[1:], workspace)
	default:
		return fmt.Errorf("unknown voice command %q", args[0])
	}
}

type voiceProviderRegistry struct {
	STT map[string]contracts.SpeechToText
	TTS map[string]contracts.TextToSpeech
}

func defaultVoiceRegistry() voiceProviderRegistry {
	return voiceProviderRegistry{
		STT: map[string]contracts.SpeechToText{
			"fake":  contractfake.NewSpeechToText(contracts.TranscriptEvent{Kind: contracts.TranscriptFinal, Text: "fake transcript"}),
			"apple": applespeechanalyzer.New(),
		},
		TTS: map[string]contracts.TextToSpeech{
			"fake":  contractfake.NewTextToSpeech(),
			"apple": appleavspeech.New(),
		},
	}
}

func runVoiceProviders(ctx context.Context, out io.Writer, registry voiceProviderRegistry) error {
	for name, provider := range registry.STT {
		caps, err := provider.Capabilities(ctx)
		if err != nil {
			fmt.Fprintf(out, "stt %s unavailable error=%q\n", name, err.Error())
			continue
		}
		fmt.Fprintf(out, "stt %s provider=%s local=%t streaming=%t offline_files=%t\n", name, caps.Provider, caps.Local, caps.Streaming, caps.OfflineFiles)
	}
	for name, provider := range registry.TTS {
		caps, err := provider.Capabilities(ctx)
		if err != nil {
			fmt.Fprintf(out, "tts %s unavailable error=%q\n", name, err.Error())
			continue
		}
		fmt.Fprintf(out, "tts %s provider=%s local=%t streaming=%t voices=%d\n", name, caps.Provider, caps.Local, caps.Streaming, len(caps.Voices))
	}
	return nil
}

func runVoiceSTTCmd(args []string) error {
	fs := flag.NewFlagSet("voice stt", flag.ContinueOnError)
	providerName := fs.String("provider", "fake", "stt provider")
	locale := fs.String("locale", "en-US", "locale")
	input := fs.String("input", "", "audio input file or mic")
	once := fs.Bool("once", false, "stop after one final transcript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	provider := defaultVoiceRegistry().STT[*providerName]
	if provider == nil {
		return fmt.Errorf("unknown stt provider %q", *providerName)
	}
	var pcm []byte
	if *input != "" && *input != "mic" {
		b, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		pcm = b
	} else {
		pcm = []byte{0}
	}
	_ = once
	_ = locale
	return runVoiceSTT(context.Background(), os.Stdout, provider, pcm)
}

func runVoiceSTT(ctx context.Context, out io.Writer, provider contracts.SpeechToText, pcm []byte) error {
	session, err := provider.OpenSTTSession(ctx, contracts.STTOptions{SessionID: "voice-stt"})
	if err != nil {
		return err
	}
	if err := session.WriteAudio(ctx, contracts.AudioFrame{SessionID: "voice-stt", PCM: pcm}); err != nil {
		return err
	}
	if err := session.Close(ctx); err != nil {
		return err
	}
	for event := range session.Events() {
		if event.Kind == contracts.TranscriptFinal {
			fmt.Fprintln(out, event.Text)
		}
	}
	return nil
}

func runVoiceTTSCmd(args []string) error {
	fs := flag.NewFlagSet("voice tts", flag.ContinueOnError)
	providerName := fs.String("provider", "fake", "tts provider")
	text := fs.String("text", "", "text to synthesize")
	voiceID := fs.String("voice", "", "voice id")
	speaker := fs.Bool("speaker", false, "play synthesized audio through the local speaker")
	savePlayback := fs.String("save-playback-pcm", "", "debug path for raw synthesized playback PCM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *text == "" {
		return fmt.Errorf("--text is required")
	}
	provider := defaultVoiceRegistry().TTS[*providerName]
	if provider == nil {
		return fmt.Errorf("unknown tts provider %q", *providerName)
	}
	var sink voice.AudioSink = &stdoutTTSSink{out: os.Stdout}
	if *speaker {
		playback, err := localaudio.NewMalgoDriver().NewPlaybackStream(context.Background(), localaudio.DefaultStreamConfig())
		if err != nil {
			return err
		}
		playbackOut, err := optionalPCMWriter(*savePlayback)
		if err != nil {
			_ = playback.Close()
			return err
		}
		defer playbackOut.Close()
		sink = &playbackTTSSink{playback: playback, debug: os.Stdout, pcmOut: playbackOut}
	}
	_ = voiceID
	return runVoiceTTS(context.Background(), provider, sink, *text)
}

func runVoiceTTS(ctx context.Context, provider contracts.TextToSpeech, sink voice.AudioSink, text string) error {
	stream, err := provider.Synthesize(ctx, contracts.SynthesisRequest{SessionID: "voice-tts", Text: text})
	if err != nil {
		return err
	}
	defer stream.Close(ctx)
	for event := range stream.Events() {
		if err := sink.WriteTTS(ctx, event); err != nil {
			return err
		}
		if event.Kind == contracts.TTSEventDone {
			return nil
		}
		if event.Kind == contracts.TTSEventError {
			return fmt.Errorf("%s", event.Error)
		}
	}
	return nil
}

type stdoutTTSSink struct {
	out io.Writer
	buf bytes.Buffer
}

func (s *stdoutTTSSink) WriteTTS(_ context.Context, event contracts.TTSEvent) error {
	if event.Kind == contracts.TTSEventAudio {
		_, _ = s.buf.Write(event.PCM)
	}
	if event.Kind == contracts.TTSEventDone {
		fmt.Fprintf(s.out, "tts audio bytes: %d\n", s.buf.Len())
	}
	return nil
}

func (s *stdoutTTSSink) Close(context.Context) error { return nil }

func runVoiceChatCmd(args []string) error {
	fs := flag.NewFlagSet("voice chat", flag.ContinueOnError)
	sttName := fs.String("stt", "fake", "stt provider")
	ttsName := fs.String("tts", "fake", "tts provider")
	locale := fs.String("locale", "en-US", "locale")
	debug := fs.Bool("debug", false, "print live voice pipeline diagnostics")
	savePlayback := fs.String("save-playback-pcm", "", "debug path for raw synthesized playback PCM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	registry := defaultVoiceRegistry()
	if registry.STT[*sttName] == nil {
		return fmt.Errorf("unknown stt provider %q", *sttName)
	}
	if registry.TTS[*ttsName] == nil {
		return fmt.Errorf("unknown tts provider %q", *ttsName)
	}
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}
	provider, err := buildProvider(cfg)
	if err != nil {
		return err
	}
	driver := localaudio.NewMalgoDriver()
	streamCfg := localaudio.DefaultStreamConfig()
	capture, err := driver.NewCaptureStream(context.Background(), streamCfg)
	if err != nil {
		return err
	}
	playback, err := driver.NewPlaybackStream(context.Background(), streamCfg)
	if err != nil {
		_ = capture.Close()
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("voice chat ready stt=%s tts=%s locale=%s\n", *sttName, *ttsName, *locale)
	var debugOut io.Writer
	if *debug {
		debugOut = os.Stdout
	}
	stt := registry.STT[*sttName]
	if *sttName == "apple" {
		stt = debouncedSpeechToText{inner: stt, delay: 900 * time.Millisecond}
	}
	err = runVoiceChat(ctx, capture, playback, stt, registry.TTS[*ttsName], &llmVoiceAgent{
		provider: provider,
		model:    cfg.modelForSource("voice"),
		out:      debugOut,
	}, *locale, voiceChatOptions{DebugOut: debugOut, SavePlaybackPCM: *savePlayback})
	if err == context.Canceled {
		fmt.Println("voice chat stopped")
		return nil
	}
	return err
}

type debouncedSpeechToText struct {
	inner contracts.SpeechToText
	delay time.Duration
}

func (d debouncedSpeechToText) Capabilities(ctx context.Context) (contracts.STTCapabilities, error) {
	return d.inner.Capabilities(ctx)
}

func (d debouncedSpeechToText) OpenSTTSession(ctx context.Context, opts contracts.STTOptions) (contracts.STTSession, error) {
	inner, err := d.inner.OpenSTTSession(ctx, opts)
	if err != nil {
		return nil, err
	}
	if d.delay <= 0 {
		d.delay = 900 * time.Millisecond
	}
	s := &debouncedSTTSession{
		inner: inner,
		delay: d.delay,
		out:   make(chan contracts.TranscriptEvent, 16),
	}
	go s.run()
	return s, nil
}

type debouncedSTTSession struct {
	inner contracts.STTSession
	delay time.Duration
	out   chan contracts.TranscriptEvent
}

func (s *debouncedSTTSession) WriteAudio(ctx context.Context, frame contracts.AudioFrame) error {
	return s.inner.WriteAudio(ctx, frame)
}

func (s *debouncedSTTSession) Events() <-chan contracts.TranscriptEvent { return s.out }

func (s *debouncedSTTSession) Close(ctx context.Context) error {
	return s.inner.Close(ctx)
}

func (s *debouncedSTTSession) run() {
	defer close(s.out)
	var (
		latest contracts.TranscriptEvent
		has    bool
		timer  *time.Timer
		timerC <-chan time.Time
	)
	flush := func() {
		if !has {
			return
		}
		s.out <- latest
		has = false
	}
	for {
		select {
		case event, ok := <-s.inner.Events():
			if !ok {
				if timer != nil {
					timer.Stop()
				}
				flush()
				return
			}
			if event.Kind != contracts.TranscriptFinal {
				s.out <- event
				continue
			}
			latest = event
			has = true
			if timer == nil {
				timer = time.NewTimer(s.delay)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(s.delay)
			}
		case <-timerC:
			flush()
			timerC = nil
			timer = nil
		}
	}
}

type voiceChatOptions struct {
	DebugOut        io.Writer
	SavePlaybackPCM string
}

func runVoiceChat(ctx context.Context, capture localaudio.CaptureStream, playback localaudio.PlaybackStream, stt contracts.SpeechToText, tts contracts.TextToSpeech, agent voice.AgentGateway, locale string, opts voiceChatOptions) error {
	playbackOut, err := optionalPCMWriter(opts.SavePlaybackPCM)
	if err != nil {
		_ = capture.Close()
		_ = playback.Close()
		return err
	}
	defer playbackOut.Close()

	sink := &playbackTTSSink{playback: playback, debug: opts.DebugOut, pcmOut: playbackOut}
	session, err := voice.NewSession(voice.SessionConfig{
		SessionID: "voice-chat",
		Locale:    locale,
		STT:       stt,
		TTS:       tts,
		Agent:     agent,
		Sink:      sink,
		State: voice.VoiceState{
			STTEnabled:              true,
			TTSEnabled:              true,
			AutoSpeakFinalResponses: true,
		},
	})
	if err != nil {
		_ = capture.Close()
		_ = playback.Close()
		return err
	}
	defer capture.Close()
	defer session.Close(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-capture.Chunks():
			if !ok {
				return nil
			}
			if len(chunk) == 0 {
				continue
			}
			if opts.DebugOut != nil {
				fmt.Fprintf(opts.DebugOut, "voice chat capture bytes=%d\n", len(chunk))
			}
			if err := session.ProcessAudio(ctx, contracts.AudioFrame{
				SessionID: "voice-chat",
				Format: contracts.AudioFormat{
					Encoding:     contracts.AudioEncodingPCM16,
					SampleRateHz: 24000,
					Channels:     1,
				},
				PCM: chunk,
			}); err != nil {
				return err
			}
		}
	}
}

type playbackTTSSink struct {
	playback localaudio.PlaybackStream
	debug    io.Writer
	pcmOut   pcmWriter
}

func (s *playbackTTSSink) WriteTTS(ctx context.Context, event contracts.TTSEvent) error {
	switch event.Kind {
	case contracts.TTSEventAudio:
		if s.debug != nil {
			fmt.Fprintf(s.debug, "voice chat tts audio bytes=%d\n", len(event.PCM))
		}
		if _, err := s.pcmOut.Write(event.PCM); err != nil {
			return err
		}
		return s.playback.WritePCM(ctx, event.PCM)
	case contracts.TTSEventDone:
		if s.debug != nil {
			fmt.Fprintln(s.debug, "voice chat tts done")
		}
		return s.playback.Drain(ctx)
	case contracts.TTSEventError:
		return fmt.Errorf("%s", event.Error)
	default:
		return nil
	}
}

func (s *playbackTTSSink) Close(context.Context) error {
	if s.playback == nil {
		return nil
	}
	return s.playback.Close()
}

type llmVoiceAgent struct {
	provider llm.Provider
	model    string
	out      io.Writer
	mu       sync.Mutex
	history  []llm.Message
}

func (a *llmVoiceAgent) SubmitVoiceText(ctx context.Context, req voice.VoiceAgentRequest) (voice.VoiceAgentResult, error) {
	if a.out != nil {
		fmt.Fprintf(a.out, "voice chat stt final=%q\n", req.Text)
	}
	a.mu.Lock()
	a.history = append(a.history, llm.Message{Role: "user", Content: req.Text})
	msgs := append([]llm.Message{{Role: "system", Content: "You are a concise voice tutor. Keep spoken replies brief."}}, a.history...)
	a.mu.Unlock()

	ch, err := a.provider.Chat(ctx, llm.Request{Model: a.model, Messages: msgs, Stream: true})
	if err != nil {
		return voice.VoiceAgentResult{}, err
	}
	var text strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return voice.VoiceAgentResult{}, chunk.Err
		}
		text.WriteString(chunk.TextDelta)
	}
	reply := text.String()
	a.mu.Lock()
	a.history = append(a.history, llm.Message{Role: "assistant", Content: reply})
	a.mu.Unlock()
	if a.out != nil {
		fmt.Fprintf(a.out, "voice chat agent reply=%q\n", reply)
	} else {
		fmt.Println(reply)
	}
	return voice.VoiceAgentResult{Text: reply}, nil
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
