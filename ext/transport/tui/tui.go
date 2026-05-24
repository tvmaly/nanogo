// Package tui implements a local Bubble Tea operator console over the gateway
// service.
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tvmaly/nanogo/modules/gateway"
)

type Config struct{}

type Model struct {
	service *gateway.Service
	active  int
	input   string
	session string
	chat    string
	status  gateway.Status
	skills  []gateway.SkillInfo
	tools   []gateway.ToolInfo
	costs   gateway.CostSummary
	events  []gateway.EventRecord
	err     string
}

type loadedMsg struct {
	status gateway.Status
	skills []gateway.SkillInfo
	tools  []gateway.ToolInfo
	costs  gateway.CostSummary
	err    error
}

type chatMsg struct {
	resp gateway.ChatResponse
	err  error
}

var tabs = []string{"Chat", "Sessions", "Skills", "Tools", "Costs", "Events"}

func NewModel(svc *gateway.Service) Model {
	return Model{service: svc, session: "tui"}
}

func Run(ctx context.Context, svc *gateway.Service) error {
	_, err := tea.NewProgram(NewModel(svc), tea.WithContext(ctx)).Run()
	return err
}

func (m Model) Init() tea.Cmd { return m.loadCmd() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.status, m.skills, m.tools, m.costs = msg.status, msg.skills, msg.tools, msg.costs
		return m, nil
	case chatMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.session = msg.resp.Session
		m.chat += "\nassistant: " + msg.resp.Text
		m.events = append(m.events, msg.resp.Events...)
		return m, m.loadCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "tab":
			m.active = (m.active + 1) % len(tabs)
			return m, nil
		case "shift+tab":
			m.active = (m.active + len(tabs) - 1) % len(tabs)
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input)
			if text == "" {
				return m, nil
			}
			m.chat += "\nuser: " + text
			m.input = ""
			return m, func() tea.Msg {
				resp, err := m.service.SubmitChat(context.Background(), gateway.ChatRequest{Session: m.session, Message: text})
				return chatMsg{resp: resp, err: err}
			}
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		default:
			if len(msg.Runes) > 0 {
				m.input += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	for i, tab := range tabs {
		label := " " + tab + " "
		if i == m.active {
			label = lipgloss.NewStyle().Bold(true).Underline(true).Render(label)
		}
		b.WriteString(label)
	}
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString("error: " + m.err + "\n")
	}
	switch tabs[m.active] {
	case "Chat":
		b.WriteString(strings.TrimSpace(m.chat))
		b.WriteString("\n> " + m.input)
	case "Sessions":
		b.WriteString(fmt.Sprintf("current: %s\nactive sessions: %d\nmodel: %s", m.session, m.status.Sessions, m.status.Model))
	case "Skills":
		for _, sk := range m.skills {
			b.WriteString(sk.Name + " " + sk.Description + "\n")
		}
	case "Tools":
		for _, tool := range m.tools {
			b.WriteString(tool.Name + "\n")
		}
	case "Costs":
		b.WriteString(fmt.Sprintf("turns=%d input=%d output=%d cached=%d usd=%.6f", m.costs.Turns, m.costs.InputTokens, m.costs.OutputTokens, m.costs.CachedInputTokens, m.costs.CostUSD))
	case "Events":
		for _, ev := range m.events {
			b.WriteString(fmt.Sprintf("%d %s %s\n", ev.Seq, ev.Event, ev.Session))
		}
	}
	return b.String()
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		skills, err := m.service.ListSkills(false)
		if err != nil {
			return loadedMsg{err: err}
		}
		tools, err := m.service.ToolCatalog(context.Background(), m.session)
		if err != nil {
			return loadedMsg{err: err}
		}
		costs, err := m.service.CostSummary(m.session)
		if err != nil {
			return loadedMsg{err: err}
		}
		return loadedMsg{status: m.service.Status(), skills: skills, tools: tools, costs: costs}
	}
}
