package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/modules/browser"
)

func runBrowserCmd(args []string, configPath, workspaceDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo browser doctor|open|snapshot|close|media-seek|open-lesson")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Browser.Enabled = true
	bus := event.NewBus()
	ctx := context.Background()
	switch args[0] {
	case "doctor":
		fs := flag.NewFlagSet("browser doctor", flag.ContinueOnError)
		driver := fs.String("driver", "", "browser driver: fake or agent-browser")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver != "" {
			cfg.Browser.Driver = *driver
		}
		svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
		if err != nil {
			return err
		}
		return printJSON(svc.Health(ctx))
	case "open", "open-lesson":
		fs := flag.NewFlagSet("browser open", flag.ContinueOnError)
		driver := fs.String("driver", "", "browser driver: fake or agent-browser")
		headed := fs.Bool("headed", false, "open a headed browser")
		sessionName := fs.String("session", "browser-cli", "browser session name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver != "" {
			cfg.Browser.Driver = *driver
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("url is required")
		}
		svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
		if err != nil {
			return err
		}
		sess, err := svc.Start(ctx, browser.StartRequest{SessionName: *sessionName, Headed: *headed})
		if err != nil {
			return err
		}
		return printJSON(svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: fs.Arg(0), WaitUntil: "load"}))
	case "snapshot":
		fs := flag.NewFlagSet("browser snapshot", flag.ContinueOnError)
		driver := fs.String("driver", "", "browser driver: fake or agent-browser")
		sessionID := fs.String("session-id", "", "browser session id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver != "" {
			cfg.Browser.Driver = *driver
		}
		if *sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}
		svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
		if err != nil {
			return err
		}
		return printJSON(svc.Snapshot(ctx, browser.SnapshotRequest{SessionID: browser.SessionID(*sessionID), InteractiveOnly: true}))
	case "close":
		fs := flag.NewFlagSet("browser close", flag.ContinueOnError)
		driver := fs.String("driver", "", "browser driver: fake or agent-browser")
		sessionID := fs.String("session-id", "", "browser session id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver != "" {
			cfg.Browser.Driver = *driver
		}
		if *sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}
		svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
		if err != nil {
			return err
		}
		return svc.Close(ctx, browser.CloseRequest{SessionID: browser.SessionID(*sessionID), CloseSession: true, Reason: "cli"})
	case "media-seek":
		fs := flag.NewFlagSet("browser media-seek", flag.ContinueOnError)
		driver := fs.String("driver", "", "browser driver: fake or agent-browser")
		sessionID := fs.String("session-id", "", "browser session id")
		seconds := fs.Float64("seconds", 0, "target media time in seconds")
		strategy := fs.String("strategy", "auto", "auto, youtube_iframe_api, html5_video, or postmessage")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *driver != "" {
			cfg.Browser.Driver = *driver
		}
		if *sessionID == "" {
			return fmt.Errorf("--session-id is required")
		}
		svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
		if err != nil {
			return err
		}
		return printJSON(svc.MediaSeek(ctx, browser.MediaSeekRequest{SessionID: browser.SessionID(*sessionID), Seconds: *seconds, Strategy: *strategy}))
	default:
		return fmt.Errorf("unknown browser command %q", args[0])
	}
}

func printJSON(v any, err error) error {
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
