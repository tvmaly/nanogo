package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/modules/browser"
)

func runBrowserCmd(args []string, configPath, workspaceDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo browser doctor|open|snapshot|close|media-seek|open-lesson|smoke-duckduckgo|smoke-youtube|smoke-youtube-iframe")
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
	case "smoke-duckduckgo", "smoke-youtube", "smoke-youtube-iframe":
		return runBrowserSmokeCmd(ctx, args, cfg, bus, workspaceDir)
	default:
		return fmt.Errorf("unknown browser command %q", args[0])
	}
}

type browserSmokeArtifact struct {
	Smoke          string `json:"smoke"`
	Driver         string `json:"driver"`
	SessionID      string `json:"session_id"`
	FinalURL       string `json:"final_url"`
	Title          string `json:"title"`
	ScreenshotPath string `json:"screenshot_path"`
	ConsoleSummary string `json:"console_summary"`
	NetworkSummary string `json:"network_summary"`
	CleanupStatus  string `json:"cleanup_status"`
	Recoverable    bool   `json:"recoverable,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func runBrowserSmokeCmd(ctx context.Context, args []string, cfg *config, bus event.Bus, workspaceDir string) error {
	name := args[0]
	fs := flag.NewFlagSet("browser "+name, flag.ContinueOnError)
	driver := fs.String("driver", "", "browser driver: fake or agent-browser")
	headed := fs.Bool("headed", false, "open a headed browser")
	seconds := fs.Int("seconds", 60, "manual observation budget in seconds")
	artifactDir := fs.String("artifact-dir", "", "directory for redacted smoke artifacts")
	rawURL := fs.String("url", "", "override smoke URL")
	yes := fs.Bool("yes", false, "skip operator confirmation")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *driver != "" {
		cfg.Browser.Driver = *driver
	}
	cfg.Browser.Enabled = true
	if *artifactDir == "" {
		*artifactDir = filepath.Join(workspaceDir, ".nanogo", "browser-smokes")
	}
	cfg.Browser.ArtifactRoot = *artifactDir
	targetURL, err := smokeURL(name, *rawURL, workspaceDir)
	if err != nil {
		return err
	}
	svc, err := buildBrowserService(cfg, bus, workspaceDir, true)
	if err != nil {
		return err
	}
	sessionName := "browser-" + strings.TrimPrefix(name, "smoke-")
	sess, err := svc.Start(ctx, browser.StartRequest{SessionName: sessionName, Headed: *headed})
	if err != nil {
		return err
	}
	cleanupStatus := "not_run"
	defer func() {
		if cleanupStatus == "closed" {
			return
		}
		_ = svc.Close(context.Background(), browser.CloseRequest{SessionID: sess.ID, CloseSession: true, Reason: "smoke_cleanup"})
	}()
	page, navErr := svc.Navigate(ctx, browser.NavigateRequest{SessionID: sess.ID, URL: targetURL, WaitUntil: "load"})
	shot, shotErr := svc.Screenshot(ctx, browser.ScreenshotRequest{SessionID: sess.ID, Path: name + ".png", FullPage: true})
	if *headed && !*yes {
		fmt.Fprintf(os.Stderr, "Inspect the headed browser for up to %d seconds, then press Enter to close the smoke session.", *seconds)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	closeErr := svc.Close(ctx, browser.CloseRequest{SessionID: sess.ID, CloseSession: true, Reason: "smoke_complete"})
	if closeErr != nil {
		cleanupStatus = "close_failed: " + browser.RedactSensitive(closeErr.Error())
	} else {
		cleanupStatus = "closed"
	}
	artifact := browserSmokeArtifact{
		Smoke:          name,
		Driver:         svc.Policy().Driver,
		SessionID:      string(sess.ID),
		FinalURL:       page.URL,
		Title:          page.Title,
		ScreenshotPath: shot.Path,
		ConsoleSummary: "not_collected",
		NetworkSummary: "not_collected",
		CleanupStatus:  cleanupStatus,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if navErr != nil {
		artifact.Recoverable = strings.Contains(name, "youtube")
		artifact.NetworkSummary = browser.RedactSensitive(navErr.Error())
	}
	if shotErr != nil {
		artifact.ConsoleSummary = browser.RedactSensitive(shotErr.Error())
	}
	artifactPath := filepath.Join(*artifactDir, name+".json")
	if err := writeSmokeArtifact(artifactPath, artifact); err != nil {
		return err
	}
	if navErr != nil {
		return navErr
	}
	if shotErr != nil {
		return shotErr
	}
	if closeErr != nil {
		return closeErr
	}
	return printJSON(artifact, nil)
}

func smokeURL(name, override, workspaceDir string) (string, error) {
	if override != "" {
		return override, nil
	}
	switch name {
	case "smoke-duckduckgo":
		return "https://duckduckgo.com/?q=nanogo+tutoring+guide", nil
	case "smoke-youtube":
		return "https://www.youtube.com/results?search_query=beginner+fractions+lesson", nil
	case "smoke-youtube-iframe":
		path := filepath.Join(workspaceDir, "lessons", "yoyo_youtube_iframe", "index.html")
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
		return u.String(), nil
	default:
		return "", fmt.Errorf("unknown browser smoke %q", name)
	}
}

func writeSmokeArtifact(path string, artifact browserSmokeArtifact) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
