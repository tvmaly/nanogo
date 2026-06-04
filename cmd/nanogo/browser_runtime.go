package main

import (
	"path/filepath"

	"github.com/tvmaly/nanogo/core/event"
	agentbrowser "github.com/tvmaly/nanogo/ext/browser/agentbrowser"
	"github.com/tvmaly/nanogo/modules/browser"
	"github.com/tvmaly/nanogo/modules/browser/fake"
)

func buildBrowserService(cfg *config, bus event.Bus, workspaceDir string, force bool) (*browser.Service, error) {
	if cfg == nil {
		return nil, nil
	}
	if !force && !cfg.Browser.Enabled {
		return nil, nil
	}
	policy := browser.Policy{
		Enabled:               true,
		Driver:                cfg.Browser.Driver,
		MaxSessions:           cfg.Browser.MaxSessions,
		SessionTTLSeconds:     cfg.Browser.SessionTTLSeconds,
		AllowedDomains:        cfg.Browser.AllowedDomains,
		AllowFileRoots:        expandPaths(cfg.Browser.AllowFileRoots),
		ArtifactRoot:          expandPath(cfg.Browser.ArtifactRoot),
		AllowEval:             cfg.Browser.AllowEval,
		AllowUploads:          cfg.Browser.AllowUploads,
		AllowDownloads:        cfg.Browser.AllowDownloads,
		AllowNonLoopbackCDP:   cfg.Browser.AllowNonLoopbackCDP,
		IncludeEvalConsole:    cfg.Browser.IncludeEvalConsole,
		SnapshotMaxDepth:      cfg.Browser.SnapshotMaxDepth,
		SnapshotMaxOutputByte: cfg.Browser.SnapshotMaxOutputBytes,
	}.WithDefaults()
	if len(policy.AllowFileRoots) == 0 && workspaceDir != "" {
		policy.AllowFileRoots = []string{filepath.Join(workspaceDir, "lessons")}
	}
	registry := expandPath(cfg.Browser.RegistryPath)
	if registry == "" && workspaceDir != "" {
		registry = filepath.Join(workspaceDir, ".nanogo", "browser-sessions.json")
	}
	var controller browser.Controller
	switch policy.Driver {
	case "", "fake":
		controller = fake.New()
	case "agent-browser":
		controller = agentbrowser.New(nil)
	}
	return browser.NewService(browser.ServiceConfig{Controller: controller, Policy: policy, Bus: bus, Registry: registry})
}

func expandPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, expandPath(p))
	}
	return out
}
