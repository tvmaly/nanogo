package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tvmaly/nanogo/core/harness"
	"github.com/tvmaly/nanogo/core/llm"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/ext/adaptive"
	"github.com/tvmaly/nanogo/modules/scheduler"
	"github.com/tvmaly/nanogo/modules/transport"
)

func TestExtensionRegistrationsByFamily(t *testing.T) {
	t.Parallel()

	assertLLMRegistered(t, "classifier-router", json.RawMessage(`{}`))
	assertLLMRegistered(t, "openai", json.RawMessage(`{}`))
	assertLLMRegistered(t, "router", json.RawMessage(`{}`))
	assertToolSourceRegistered(t, "adaptive", json.RawMessage(`{"root":"`+t.TempDir()+`"}`))
	assertToolSourceRegistered(t, "tutorruntime", json.RawMessage(`{"root":"`+t.TempDir()+`"}`))
	assertContainsAll(t, "transports", transport.Registered(), []string{"cli", "repl", "rest", "webui"})
	if _, err := scheduler.Build("stdlib", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("scheduler registration stdlib: %v", err)
	}
	assertContainsAll(t, "harness sensors", harness.AllSensorNames(), []string{"context_guard", "file_changed", "gotest", "vet"})
	assertContainsAll(t, "adaptive domains", adaptive.RegisteredDomains(), []string{"tutorruntime"})
}

func assertLLMRegistered(t *testing.T, name string, cfg json.RawMessage) {
	t.Helper()
	if _, err := llm.Build(name, cfg); errors.Is(err, llm.ErrUnknownDriver) {
		t.Fatalf("llm provider %q is not registered: %v", name, err)
	}
}

func assertToolSourceRegistered(t *testing.T, name string, cfg json.RawMessage) {
	t.Helper()
	if _, err := tools.Build(name, cfg); err != nil {
		t.Fatalf("tool source %q is not registered: %v", name, err)
	}
}

func assertContainsAll(t *testing.T, family string, got []string, want []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, name := range got {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("%s registrations = %v, missing %q", family, got, name)
		}
	}
}
