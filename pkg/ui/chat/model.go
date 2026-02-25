package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	providertypes "miniclaw/pkg/provider/types"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeInteractive mode = iota
	modeOneShot
)

type chatMessage struct {
	role    string
	content string
	usage   *providertypes.TokenUsage
}

type promptResultMsg struct {
	result providertypes.PromptResult
	err    error
}

type bootTickMsg struct{}

type model struct {
	ctx          context.Context
	promptFn     PromptFunc
	mode         mode
	oneShotInput string

	theme      theme
	spinner    spinner.Model
	input      textinput.Model
	viewport   viewport.Model
	messages   []chatMessage
	width      int
	height     int
	isReady    bool
	isLoading  bool
	lastErr    string
	booting    bool
	bootStep   int
	followLog  bool
	runtime    RuntimeInfo
	usageIn    int64
	usageOut   int64
	usageTotal int64
}

func newModel(ctx context.Context, promptFn PromptFunc, runMode mode, prompt string, info RuntimeInfo) *model {
	spin := spinner.New()
	spin.Spinner = spinner.Points
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "Ask anything..."
	in.Focus()
	in.CharLimit = 0

	vp := viewport.New(80, 12)

	return &model{
		ctx:          ctx,
		promptFn:     promptFn,
		mode:         runMode,
		oneShotInput: strings.TrimSpace(prompt),
		theme:        defaultTheme(),
		spinner:      spin,
		input:        in,
		viewport:     vp,
		width:        100,
		height:       28,
		booting:      runMode == modeInteractive,
		followLog:    true,
		runtime:      info,
	}
}

func (m *model) Init() tea.Cmd {
	if m.mode == modeOneShot && m.oneShotInput != "" {
		m.messages = append(m.messages, chatMessage{role: "user", content: m.oneShotInput})
		m.isLoading = true
		m.refreshViewport(false)
		return tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, m.oneShotInput))
	}

	return bootTickCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeComponents()
		m.refreshViewport(false)
		m.isReady = true
		return m, nil
	case bootTickMsg:
		if !m.booting {
			return m, nil
		}

		m.bootStep++
		if m.bootStep < len(bootScriptLines())+1 {
			return m, bootTickCmd()
		}

		m.booting = false
		if m.mode == modeOneShot && m.oneShotInput != "" {
			m.messages = append(m.messages, chatMessage{role: "user", content: m.oneShotInput})
			m.isLoading = true
			m.refreshViewport(false)
			return m, tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, m.oneShotInput))
		}

		if m.mode == modeInteractive {
			return m, textinput.Blink
		}

		return m, nil
	case tea.KeyMsg:
		switch typed.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}

		if m.booting {
			return m, nil
		}

		if m.mode == modeInteractive {
			if handled := m.handleViewportKey(typed); handled {
				return m, nil
			}
		}

		if m.mode == modeOneShot {
			return m, nil
		}

		if typed.String() == "enter" {
			if m.isLoading {
				return m, nil
			}

			prompt := strings.TrimSpace(m.input.Value())
			if prompt == "" {
				return m, nil
			}
			if isExitCommand(prompt) {
				return m, tea.Quit
			}

			m.lastErr = ""
			m.messages = append(m.messages, chatMessage{role: "user", content: prompt})
			m.input.SetValue("")
			m.isLoading = true
			m.followLog = true
			m.refreshViewport(true)
			return m, tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, prompt))
		}
	}

	if m.mode == modeInteractive {
		m.input, cmd = m.input.Update(msg)
	}

	switch typed := msg.(type) {
	case spinner.TickMsg:
		if !m.isLoading {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(typed)
		return m, cmd
	case promptResultMsg:
		m.isLoading = false
		if typed.err != nil {
			m.lastErr = typed.err.Error()
			m.messages = append(m.messages, chatMessage{role: "error", content: typed.err.Error()})
		} else {
			m.lastErr = ""
			m.messages = append(m.messages, chatMessage{role: "assistant", content: typed.result.Text, usage: typed.result.Metadata.Usage})
			if typed.result.Metadata.Usage != nil {
				m.usageIn += typed.result.Metadata.Usage.InputTokens
				m.usageOut += typed.result.Metadata.Usage.OutputTokens
				m.usageTotal += typed.result.Metadata.Usage.TotalTokens
			}
		}
		m.refreshViewport(false)
		if m.mode == modeOneShot {
			return m, tea.Quit
		}
	}

	return m, cmd
}

func (m *model) View() string {
	if !m.isReady {
		m.resizeComponents()
		m.refreshViewport(false)
	}
	if m.mode == modeOneShot {
		return m.oneShotView()
	}
	if m.booting {
		return m.bootView()
	}

	header := m.theme.header.Width(m.width - 2).Render("📟 MiniClaw Command Center")
	meta := m.theme.headerMeta.Render(fmt.Sprintf(
		"agent:%s · provider:%s · model:%s · turns:%d · tokens(in/out/total):%d/%d/%d",
		displayOrNA(m.runtime.AgentType),
		displayOrNA(m.runtime.Provider),
		displayOrNA(m.runtime.Model),
		conversationTurns(m.messages),
		m.usageIn,
		m.usageOut,
		m.usageTotal,
	))
	line := m.theme.divider.Width(m.width - 2).Render(strings.Repeat("═", max(8, m.width-2)))

	status := m.theme.status.Render("💡 Enter send  ·  PgUp/PgDn scroll  ·  End jump latest  ·  🛑 Ctrl+C/Esc quit")
	if m.isLoading {
		status = m.theme.statusBusy.Render(fmt.Sprintf("%s ⚡ generating response...", m.spinner.View()))
	}
	if m.lastErr != "" {
		status = m.theme.statusErr.Render("🚨 last request failed - try again")
	}

	parts := []string{header, meta, line, m.theme.viewport.Width(m.width - 2).Render(m.viewport.View()), status}

	if m.mode == modeInteractive {
		parts = append(parts,
			m.theme.inputLabel.Render("👨🏻 You")+" "+m.theme.hint.Render("(type /exit, quit, or :q)"),
			m.theme.input.Width(m.width-2).Render(m.input.View()),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *model) resizeComponents() {
	w := m.width - 6
	if w < 50 {
		w = 50
	}
	h := m.height - 10
	if m.mode == modeOneShot {
		h = m.height - 6
	}
	if h < 8 {
		h = 8
	}

	m.viewport.Width = w
	m.viewport.Height = h
	m.input.Width = w - 2
}

func (m *model) refreshViewport(forceBottom bool) {
	previousOffset := m.viewport.YOffset
	var sections []string
	for _, item := range m.messages {
		switch item.role {
		case "user":
			sections = append(sections, m.renderCard(
				m.theme.userTitle.Render("▛▚ [ 👨🏻 ] ▞▜"),
				m.theme.userBox.Width(m.viewport.Width).Render(strings.TrimSpace(item.content)),
			))
		case "assistant":
			assistantBody := strings.TrimSpace(item.content)
			if item.usage != nil {
				assistantBody = strings.TrimSpace(assistantBody + "\n\n" + m.theme.hint.Render(formatUsageLine(*item.usage)))
			}
			sections = append(sections, m.renderCard(
				m.theme.assistantTitle.Render("▛▚ [ 🦞 ] ▞▜"),
				m.theme.assistantBox.Width(m.viewport.Width).Render(assistantBody),
			))
		case "error":
			sections = append(sections, m.renderCard(
				m.theme.errorTitle.Render("▛▚ [ERROR] ▞▜"),
				m.theme.errorBox.Width(m.viewport.Width).Render(strings.TrimSpace(item.content)),
			))
		}
	}

	m.viewport.SetContent(strings.Join(sections, "\n\n"))
	if m.followLog || forceBottom {
		m.viewport.GotoBottom()
		m.followLog = true
		return
	}

	maxOffset := m.viewport.TotalLineCount() - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if previousOffset > maxOffset {
		previousOffset = maxOffset
	}
	m.viewport.SetYOffset(previousOffset)
}

func (m *model) renderCard(title string, body string) string {
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

func (m *model) oneShotView() string {
	contentWidth := max(40, m.width-6)
	parts := []string{m.renderCard(
		m.theme.userTitle.Render("▛▚ [SENT] ▞▜"),
		m.theme.userBox.Width(contentWidth).Render(strings.TrimSpace(m.oneShotInput)),
	)}

	if m.isLoading {
		parts = append(parts, m.theme.statusBusy.Render(fmt.Sprintf("%s ⚡ sending prompt and waiting for answer...", m.spinner.View())))
		return lipgloss.JoinVertical(lipgloss.Left, parts...) + "\n"
	}

	if m.lastErr != "" {
		parts = append(parts,
			m.renderCard(
				m.theme.errorTitle.Render("▛▚ [ERROR] ▞▜"),
				m.theme.errorBox.Width(contentWidth).Render(strings.TrimSpace(m.lastErr)),
			),
		)
		return lipgloss.JoinVertical(lipgloss.Left, parts...) + "\n\n"
	}

	answer := ""
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant" {
			answer = m.messages[i].content
			break
		}
	}

	parts = append(parts,
		m.renderCard(
			m.theme.assistantTitle.Render("▛▚ [ANSWER] ▞▜"),
			m.theme.assistantBox.Width(contentWidth).Render(strings.TrimSpace(answer)),
		),
	)

	return lipgloss.JoinVertical(lipgloss.Left, parts...) + "\n\n"
}

func (m *model) bootView() string {
	header := m.theme.header.Width(m.width - 2).Render("📟 MiniClaw Command Center")
	meta := m.theme.headerMeta.Render("boot sequence")
	line := m.theme.divider.Width(m.width - 2).Render(strings.Repeat("═", max(8, m.width-2)))

	script := bootScriptLines()
	count := min(m.bootStep, len(script))
	visible := make([]string, 0, count+1)
	for i := 0; i < count; i++ {
		visible = append(visible, m.theme.bootLine.Render(script[i]))
	}
	if m.bootStep > len(script) {
		visible = append(visible, m.theme.bootDone.Render("✅ command center online"))
	}

	body := m.theme.viewport.Width(m.width - 2).Render(strings.Join(visible, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, header, meta, line, body)
}

func max(a int, b int) int {
	if a > b {
		return a
	}

	return b
}

func min(a int, b int) int {
	if a < b {
		return a
	}

	return b
}

func bootTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return bootTickMsg{}
	})
}

func (m *model) handleViewportKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "ctrl+b", "alt+up", "ctrl+up":
		m.viewport.PageUp()
		m.followLog = false
		return true
	case "pgdown", "ctrl+f", "alt+down", "ctrl+down":
		m.viewport.PageDown()
		if m.viewport.AtBottom() {
			m.followLog = true
		}
		return true
	case "home":
		m.viewport.GotoTop()
		m.followLog = false
		return true
	case "end":
		m.viewport.GotoBottom()
		m.followLog = true
		return true
	default:
		return false
	}
}

func bootScriptLines() []string {
	return []string{
		"[BOOT] power rails stable",
		"[BOOT] loading retro renderer",
		"[BOOT] calibrating prompt bus",
		"[BOOT] syncing lobster core",
	}
}

func sendPromptCmd(ctx context.Context, promptFn PromptFunc, prompt string) tea.Cmd {
	return func() tea.Msg {
		result, err := promptFn(ctx, prompt)
		return promptResultMsg{result: result, err: err}
	}
}

func displayOrNA(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "n/a"
	}

	return trimmed
}

func conversationTurns(messages []chatMessage) int {
	count := 0
	for _, message := range messages {
		if message.role == "user" {
			count++
		}
	}

	return count
}

func formatUsageLine(usage providertypes.TokenUsage) string {
	return fmt.Sprintf("tokens in/out/total: %d/%d/%d", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
}

func isExitCommand(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "exit", "/exit", "quit", ":q":
		return true
	default:
		return false
	}
}
