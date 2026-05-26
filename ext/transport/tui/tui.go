// Package tui implements a local Bubble Tea operator console over the gateway
// service.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tvmaly/nanogo/modules/gateway"
	"github.com/tvmaly/nanogo/modules/help"
)

type Config struct {
	EventLimit int
}

type Model struct {
	service *gateway.Service
	cfg     Config

	active          int
	input           string
	session         string
	selectedSession int
	selectedSkill   int
	status          gateway.Status
	sessions        []gateway.SessionInfo
	skills          []gateway.SkillInfo
	tools           []gateway.ToolInfo
	costs           gateway.CostSummary
	events          []gateway.EventRecord
	chat            []string
	streaming       bool
	streamBuffer    string
	err             string
	helpOpen        bool
	helpTitle       string
	helpText        string
	helpResults     []help.SearchHit
	selectedHelp    int
	width           int
	height          int
	chatScroll      int
}

type loadedMsg struct {
	status   gateway.Status
	sessions []gateway.SessionInfo
	skills   []gateway.SkillInfo
	tools    []gateway.ToolInfo
	costs    gateway.CostSummary
	err      error
}

type chatMsg struct {
	resp gateway.ChatResponse
	err  error
}

type commandMsg struct {
	text string
	err  error
}

type helpMsg struct {
	title   string
	text    string
	results []help.SearchHit
	err     error
}

type streamDeltaMsg struct{ delta string }
type streamDoneMsg struct {
	text string
	err  error
}

var tabs = []string{"Chat", "Sessions", "Skills", "Tools", "Costs", "Events"}

func NewModel(svc *gateway.Service) Model {
	return NewModelWithConfig(svc, Config{})
}

func NewModelWithConfig(svc *gateway.Service, cfg Config) Model {
	if cfg.EventLimit <= 0 {
		cfg.EventLimit = 100
	}
	return Model{service: svc, cfg: cfg, session: "tui"}
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
		m.err = ""
		m.status, m.sessions, m.skills, m.tools, m.costs = msg.status, msg.sessions, msg.skills, msg.tools, msg.costs
		return m, nil
	case chatMsg:
		m.streaming = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.session = msg.resp.Session
		m.chat = append(m.chat, "assistant: "+msg.resp.Text)
		m.chatScroll = 0
		m.appendEvents(msg.resp.Events...)
		return m, m.loadCmd()
	case commandMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.chat = append(m.chat, "error: "+msg.err.Error())
			return m, nil
		}
		m.err = ""
		if msg.text != "" {
			m.chat = append(m.chat, msg.text)
		}
		return m, m.loadCmd()
	case helpMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.helpOpen = true
		m.helpTitle = msg.title
		m.helpText = msg.text
		m.helpResults = append([]help.SearchHit(nil), msg.results...)
		m.selectedHelp = 0
		return m, nil
	case streamDeltaMsg:
		m.streaming = true
		m.streamBuffer += msg.delta
		return m, nil
	case streamDoneMsg:
		m.streaming = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		text := msg.text
		if text == "" {
			text = m.streamBuffer
		}
		m.streamBuffer = ""
		m.chat = append(m.chat, "assistant: "+text)
		m.chatScroll = 0
		return m, m.loadCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampChatScroll()
		return m, nil
	case tea.KeyMsg:
		if (m.helpOpen || tabs[m.active] == "Chat") && len(msg.Runes) > 0 {
			m.input += string(msg.Runes)
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.helpOpen {
				m.helpOpen = false
				return m, nil
			}
			return m, tea.Quit
		case "tab":
			m.active = (m.active + 1) % len(tabs)
			return m, nil
		case "shift+tab":
			m.active = (m.active + len(tabs) - 1) % len(tabs)
			return m, nil
		case "up":
			if m.helpOpen {
				m.selectedHelp = clampSelection(m.selectedHelp-1, len(m.helpResults))
			} else if tabs[m.active] == "Chat" {
				m.scrollChat(1)
			} else {
				m.moveSelection(-1)
			}
			return m, nil
		case "down":
			if m.helpOpen {
				m.selectedHelp = clampSelection(m.selectedHelp+1, len(m.helpResults))
			} else if tabs[m.active] == "Chat" {
				m.scrollChat(-1)
			} else {
				m.moveSelection(1)
			}
			return m, nil
		case "pgup":
			if tabs[m.active] == "Chat" && !m.helpOpen {
				m.scrollChat(m.chatPageSize())
			}
			return m, nil
		case "pgdown":
			if tabs[m.active] == "Chat" && !m.helpOpen {
				m.scrollChat(-m.chatPageSize())
			}
			return m, nil
		case "home":
			if tabs[m.active] == "Chat" && !m.helpOpen {
				m.chatScroll = m.maxChatScroll()
			}
			return m, nil
		case "end":
			if tabs[m.active] == "Chat" && !m.helpOpen {
				m.chatScroll = 0
			}
			return m, nil
		case "n":
			if tabs[m.active] == "Sessions" {
				return m, m.createSessionCmd()
			}
		case "d":
			if tabs[m.active] == "Sessions" {
				return m, m.deleteSelectedSessionCmd()
			}
		case "r":
			return m, m.loadCmd()
		case "enter":
			if m.helpOpen && len(m.helpResults) > 0 && m.input == "" {
				return m, m.helpTopicCmd(m.helpResults[m.selectedHelp].ID)
			}
			if tabs[m.active] == "Skills" {
				return m, m.runSelectedSkillCmd()
			}
			if tabs[m.active] == "Sessions" {
				m.selectSession()
				return m, nil
			}
			text := strings.TrimSpace(m.input)
			if text == "" {
				return m, nil
			}
			m.input = ""
			if strings.HasPrefix(text, "/") {
				if text == "/exit" || text == "/quit" {
					return m, tea.Quit
				}
				return m, m.slashCmd(text)
			}
			m.chat = append(m.chat, "user: "+text)
			m.chatScroll = 0
			m.streaming = true
			return m, m.chatCmd(text)
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
	if m.helpOpen {
		b.WriteString("Help: " + m.helpTitle + "\n")
		if len(m.helpResults) > 0 {
			for i, hit := range m.helpResults {
				prefix := "  "
				if i == m.selectedHelp {
					prefix = "> "
				}
				b.WriteString(prefix + hit.ID + " - " + hit.Summary + "\n")
			}
		}
		if m.helpText != "" {
			b.WriteString(m.helpText)
			if !strings.HasSuffix(m.helpText, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("> " + m.input)
		return b.String()
	}
	switch tabs[m.active] {
	case "Chat":
		lines := m.chatLines()
		if len(lines) == 0 {
			b.WriteString("No chat yet.\n")
		} else {
			b.WriteString(strings.Join(m.visibleChatLines(lines), "\n"))
			b.WriteString("\n")
		}
		b.WriteString("> " + m.input)
	case "Sessions":
		b.WriteString(fmt.Sprintf("current: %s\nactive sessions: %d\nmodel: %s\n", m.session, len(m.sessions), m.status.Model))
		if len(m.sessions) == 0 {
			b.WriteString("No sessions.\n")
		}
		for i, sess := range m.sessions {
			prefix := "  "
			if i == m.selectedSession {
				prefix = "> "
			}
			b.WriteString(prefix + sess.ID + "\n")
		}
	case "Skills":
		if len(m.skills) == 0 {
			b.WriteString("No skills.\n")
		}
		for i, sk := range m.skills {
			prefix := "  "
			if i == m.selectedSkill {
				prefix = "> "
			}
			b.WriteString(prefix + sk.Name + " " + sk.Description + "\n")
		}
	case "Tools":
		if len(m.tools) == 0 {
			b.WriteString("No tools.\n")
		}
		for _, tool := range m.tools {
			b.WriteString(tool.Name + "\n")
		}
	case "Costs":
		b.WriteString(formatCost(m.costs))
	case "Events":
		if len(m.events) == 0 {
			b.WriteString("No events.\n")
		}
		for _, ev := range m.events {
			b.WriteString(fmt.Sprintf("%d %s %s %v\n", ev.Seq, ev.Event, ev.Session, ev.Payload))
		}
	}
	return b.String()
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		sessions := m.service.ListSessions()
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
		return loadedMsg{status: m.service.Status(), sessions: sessions, skills: skills, tools: tools, costs: costs}
	}
}

func (m Model) chatCmd(text string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.service.SubmitChat(context.Background(), gateway.ChatRequest{Session: m.session, Message: text})
		return chatMsg{resp: resp, err: err}
	}
}

func (m Model) slashCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if strings.HasPrefix(text, "/help") {
			return m.runHelpSlash(context.Background(), text)
		}
		out, err := m.runSlash(context.Background(), text)
		return commandMsg{text: out, err: err}
	}
}

func (m Model) runHelpSlash(ctx context.Context, text string) helpMsg {
	fields := strings.Fields(text)
	if len(fields) == 1 {
		raw, _ := json.Marshal(help.SuggestRequest{Interface: "tui.chat", Limit: 5})
		payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "help.suggest", Params: raw})
		if err != nil {
			return helpMsg{err: err}
		}
		resp := payload.(help.SuggestResponse)
		return helpMsg{title: "Suggestions", results: resp.Hits}
	}
	if len(fields) == 2 && fields[1] == "validate" {
		payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "help.validate"})
		if err != nil {
			return helpMsg{err: err}
		}
		resp := payload.(help.ValidateResponse)
		if resp.OK {
			return helpMsg{title: "Validation", text: "help validation: ok\n"}
		}
		return helpMsg{title: "Validation", text: "help validation errors:\n" + strings.Join(resp.Errors, "\n")}
	}
	if len(fields) == 3 && fields[1] == "topic" {
		return m.openHelpTopic(ctx, fields[2])
	}
	query := strings.TrimSpace(strings.TrimPrefix(text, "/help"))
	raw, _ := json.Marshal(help.SearchRequest{Query: query, Interface: "tui.chat", Limit: 5})
	payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "help.search", Params: raw})
	if err != nil {
		return helpMsg{err: err}
	}
	resp := payload.(help.SearchResponse)
	return helpMsg{title: "Search: " + query, results: resp.Hits}
}

func (m Model) helpTopicCmd(id string) tea.Cmd {
	return func() tea.Msg {
		return m.openHelpTopic(context.Background(), id)
	}
}

func (m Model) openHelpTopic(ctx context.Context, id string) helpMsg {
	raw, _ := json.Marshal(help.TopicRequest{ID: id, Interface: "tui.chat"})
	payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "help.topic", Params: raw})
	if err != nil {
		return helpMsg{err: err}
	}
	resp := payload.(help.TopicResponse)
	rendered, err := m.service.Dispatch(ctx, gateway.Request{Method: "help.render", Params: mustJSON(help.RenderRequest{TopicID: resp.Topic.ID, Format: help.FormatTUI, Width: 80})})
	if err != nil {
		return helpMsg{err: err}
	}
	return helpMsg{title: resp.Topic.Title, text: rendered.(help.RenderResponse).Text}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (m Model) runSlash(ctx context.Context, text string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", nil
	}
	switch fields[0] {
	case "/cost":
		costs, err := m.service.CostSummary(m.session)
		if err != nil {
			return "", err
		}
		return "cost: " + formatCost(costs), nil
	case "/model":
		return m.runModelSlash(ctx, fields)
	case "/exit", "/quit":
		return "exiting", nil
	case "/stt", "/tts":
		if len(fields) != 2 || (fields[1] != "on" && fields[1] != "off") {
			return "", fmt.Errorf("usage: %s on|off", fields[0])
		}
		enabled := fields[1] == "on"
		req := map[string]any{"session": m.session}
		if fields[0] == "/stt" {
			req["stt_enabled"] = enabled
		} else {
			req["tts_enabled"] = enabled
		}
		raw, _ := json.Marshal(req)
		payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "voice.update", Params: raw})
		if err != nil {
			return "", err
		}
		state := payload.(gateway.VoiceState)
		return fmt.Sprintf("voice: stt=%t tts=%t", state.STTEnabled, state.TTSEnabled), nil
	case "/xai":
		return m.runXAISlash(ctx, fields)
	default:
		return "", fmt.Errorf("unknown command %s", fields[0])
	}
}

func (m Model) runModelSlash(ctx context.Context, fields []string) (string, error) {
	if len(fields) < 2 {
		return "", fmt.Errorf("usage: /model current|list|search|use|flush")
	}
	switch fields[1] {
	case "current":
		raw, _ := json.Marshal(map[string]string{"session": m.session})
		payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "models.current", Params: raw})
		if err != nil {
			return "", err
		}
		state := payload.(gateway.ModelState)
		return "model: " + state.Model, nil
	case "list":
		payload, err := m.service.Dispatch(ctx, gateway.Request{Method: "models.list"})
		if err != nil {
			return "", err
		}
		models := payload.([]gateway.ModelInfo)
		var ids []string
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		if len(ids) == 0 {
			return "models: none", nil
		}
		return "models:\n" + strings.Join(ids, "\n"), nil
	case "search":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /model search <partial-name>")
		}
		models, err := m.service.ListModels(ctx)
		if err != nil {
			return "", err
		}
		ids := matchingModelSuffixes(models, fields[2])
		if len(ids) == 0 {
			return "models: none", nil
		}
		return "models:\n" + strings.Join(ids, "\n"), nil
	case "flush":
		_, err := m.service.Dispatch(ctx, gateway.Request{Method: "models.flush"})
		if err != nil {
			return "", err
		}
		return "model cache flushed", nil
	case "use":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /model use <model-id>")
		}
		modelID := fields[2]
		models, err := m.service.ListModels(ctx)
		if err != nil {
			return "", err
		}
		found := false
		for _, model := range models {
			if model.ID == modelID {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("invalid model %q", modelID)
		}
		raw, _ := json.Marshal(map[string]string{"session": m.session, "model": modelID})
		if _, err := m.service.Dispatch(ctx, gateway.Request{Method: "models.use", Params: raw}); err != nil {
			return "", err
		}
		return "model set: " + modelID, nil
	default:
		return "", fmt.Errorf("usage: /model current|list|search|use|flush")
	}
}

func matchingModelSuffixes(models []gateway.ModelInfo, partial string) []string {
	partial = strings.ToLower(strings.TrimSpace(partial))
	if partial == "" {
		return nil
	}
	var ids []string
	for _, model := range models {
		suffix := model.ID
		if i := strings.LastIndex(suffix, "/"); i >= 0 {
			suffix = suffix[i+1:]
		}
		if strings.HasPrefix(strings.ToLower(suffix), partial) {
			ids = append(ids, model.ID)
		}
	}
	return ids
}

func (m Model) runXAISlash(ctx context.Context, fields []string) (string, error) {
	if len(fields) != 2 {
		return "", fmt.Errorf("usage: /xai on|off|status")
	}
	method := map[string]string{"on": "xai.start", "off": "xai.stop", "status": "xai.status"}[fields[1]]
	if method == "" {
		return "", fmt.Errorf("usage: /xai on|off|status")
	}
	payload, err := m.service.Dispatch(ctx, gateway.Request{Method: method})
	if err != nil {
		return "", err
	}
	state := payload.(gateway.RealtimeVoiceState)
	return fmt.Sprintf("xai: provider=%s model=%s session=%s connected=%t", state.Provider, state.Model, state.SessionID, state.Connected), nil
}

func (m Model) createSessionCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := m.service.CreateSession("")
		if err != nil {
			return commandMsg{err: err}
		}
		return commandMsg{text: "session created"}
	}
}

func (m Model) deleteSelectedSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if len(m.sessions) == 0 || m.selectedSession >= len(m.sessions) {
			return commandMsg{text: "no session selected"}
		}
		id := m.sessions[m.selectedSession].ID
		if id == m.session {
			return commandMsg{text: "cannot delete current session"}
		}
		if err := m.service.DeleteSession(id); err != nil {
			return commandMsg{err: err}
		}
		return commandMsg{text: "session deleted: " + id}
	}
}

func (m Model) runSelectedSkillCmd() tea.Cmd {
	return func() tea.Msg {
		if len(m.skills) == 0 || m.selectedSkill >= len(m.skills) {
			return commandMsg{text: "no skill selected"}
		}
		name := m.skills[m.selectedSkill].Name
		if err := m.service.RunSkill(context.Background(), gateway.RunSkillRequest{Name: name, Session: m.session}); err != nil {
			return commandMsg{err: err}
		}
		return commandMsg{text: "skill ran: " + name}
	}
}

func (m *Model) moveSelection(delta int) {
	switch tabs[m.active] {
	case "Sessions":
		m.selectedSession = clampSelection(m.selectedSession+delta, len(m.sessions))
	case "Skills":
		m.selectedSkill = clampSelection(m.selectedSkill+delta, len(m.skills))
	}
}

func (m *Model) selectSession() {
	if len(m.sessions) == 0 || m.selectedSession >= len(m.sessions) {
		return
	}
	m.session = m.sessions[m.selectedSession].ID
}

func (m *Model) appendEvents(events ...gateway.EventRecord) {
	m.events = append(m.events, events...)
	if len(m.events) > m.cfg.EventLimit {
		m.events = append([]gateway.EventRecord(nil), m.events[len(m.events)-m.cfg.EventLimit:]...)
	}
}

func (m Model) chatLines() []string {
	var lines []string
	for _, entry := range m.chat {
		lines = append(lines, strings.Split(entry, "\n")...)
	}
	if m.streaming {
		lines = append(lines, strings.Split("assistant: "+m.streamBuffer, "\n")...)
	}
	return lines
}

func (m Model) visibleChatLines(lines []string) []string {
	limit := m.chatBodyHeight()
	if limit <= 0 || len(lines) <= limit {
		return lines
	}
	maxScroll := len(lines) - limit
	scroll := m.chatScroll
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	start := len(lines) - limit - scroll
	return lines[start : start+limit]
}

func (m Model) chatBodyHeight() int {
	if m.height <= 0 {
		return 0
	}
	used := 2
	if m.err != "" {
		used++
	}
	height := m.height - used
	if height < 1 {
		return 1
	}
	return height
}

func (m Model) chatPageSize() int {
	size := m.chatBodyHeight() - 1
	if size < 1 {
		return 1
	}
	return size
}

func (m Model) maxChatScroll() int {
	lines := m.chatLines()
	limit := m.chatBodyHeight()
	if limit <= 0 || len(lines) <= limit {
		return 0
	}
	return len(lines) - limit
}

func (m *Model) scrollChat(delta int) {
	m.chatScroll += delta
	m.clampChatScroll()
}

func (m *Model) clampChatScroll() {
	maxScroll := m.maxChatScroll()
	if m.chatScroll < 0 {
		m.chatScroll = 0
	}
	if m.chatScroll > maxScroll {
		m.chatScroll = maxScroll
	}
}

func clampSelection(n, length int) int {
	if length <= 0 {
		return 0
	}
	if n < 0 {
		return length - 1
	}
	if n >= length {
		return 0
	}
	return n
}

func formatCost(c gateway.CostSummary) string {
	return fmt.Sprintf("turns=%d input=%d output=%d cached=%d usd=%.6f unknown=%d", c.Turns, c.InputTokens, c.OutputTokens, c.CachedInputTokens, c.CostUSD, c.UnknownCostRecords)
}
