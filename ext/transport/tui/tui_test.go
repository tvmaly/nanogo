package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tvmaly/nanogo/core/event"
	"github.com/tvmaly/nanogo/core/llm"
	fakellm "github.com/tvmaly/nanogo/core/llm/fake"
	"github.com/tvmaly/nanogo/core/session"
	"github.com/tvmaly/nanogo/core/tools"
	"github.com/tvmaly/nanogo/modules/gateway"
)

type source struct{}

func (source) Tools(context.Context, tools.TurnInfo) ([]tools.Tool, error) { return nil, nil }

func TestModelLoadsAndViewsPanes(t *testing.T) {
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{Provider: provider, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}, Model: "m"})
	m := NewModel(svc)
	msg := m.loadCmd()().(loadedMsg)
	next, _ := m.Update(msg)
	m = next.(Model)
	for range tabs {
		view := m.View()
		if !strings.Contains(view, tabs[m.active]) {
			t.Fatalf("view missing active tab: %s", view)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
	}
}

func TestModelChatUpdate(t *testing.T) {
	provider := fakellm.New([]llm.Chunk{{TextDelta: "ok"}, {FinishReason: "stop"}})
	svc := gateway.New(gateway.Config{Provider: provider, Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus(), Source: source{}})
	m := NewModel(svc)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("enter did not produce chat command")
	}
	msg := cmd().(chatMsg)
	next, _ = m.Update(msg)
	m = next.(Model)
	if !strings.Contains(m.View(), "assistant: ok") {
		t.Fatalf("view = %s", m.View())
	}
}

func TestModelRendersGatewayErrors(t *testing.T) {
	svc := gateway.New(gateway.Config{Store: session.NewStore(t.TempDir(), nil), Bus: event.NewBus()})
	m := NewModel(svc)
	msg := m.loadCmd()().(loadedMsg)
	if msg.err == nil {
		t.Fatal("expected load error from missing tool source")
	}
	next, _ := m.Update(msg)
	m = next.(Model)
	if !strings.Contains(m.View(), "error:") {
		t.Fatalf("view = %s", m.View())
	}
	next, _ = m.Update(chatMsg{err: context.Canceled})
	m = next.(Model)
	if !strings.Contains(m.View(), "context canceled") {
		t.Fatalf("view = %s", m.View())
	}
}
