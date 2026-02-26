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

type toolEventMsg struct {
	event  providertypes.ToolEvent
	stream <-chan providertypes.ToolEvent
}

type toolEventStreamClosedMsg struct{}

type bootTickMsg struct{}

// model is the Bubble Tea state container for chat UI rendering and interaction.
type model struct {
	ctx          context.Context
	promptFn     PromptFunc
	mode         mode
	oneShotInput string

	theme                   theme
	spinner                 spinner.Model
	input                   textinput.Model
	viewport                viewport.Model
	messages                []chatMessage
	width                   int
	height                  int
	isReady                 bool
	isLoading               bool
	lastErr                 string
	booting                 bool
	bootStep                int
	followLog               bool
	showTools               bool
	pendingToolMessageIndex int
	receivedLiveToolEvents  bool
	runtime                 RuntimeInfo
	usageIn                 int64
	usageOut                int64
	usageTotal              int64
}

// newModel initializes chat UI state for interactive or one-shot mode.
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
		ctx:                     ctx,
		promptFn:                promptFn,
		mode:                    runMode,
		oneShotInput:            strings.TrimSpace(prompt),
		theme:                   defaultTheme(),
		spinner:                 spin,
		input:                   in,
		viewport:                vp,
		width:                   100,
		height:                  28,
		booting:                 runMode == modeInteractive,
		followLog:               true,
		showTools:               true,
		pendingToolMessageIndex: -1,
		runtime:                 info,
	}
}

func (m *model) Init() tea.Cmd {
	if m.mode == modeOneShot && m.oneShotInput != "" {
		m.messages = append(m.messages, chatMessage{role: "user", content: m.oneShotInput})
		m.isLoading = true
		m.refreshViewport(false)
		toolStream := make(chan providertypes.ToolEvent, 16)
		m.pendingToolMessageIndex = -1
		m.receivedLiveToolEvents = false
		return tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, m.oneShotInput, toolStream), waitToolEventCmd(toolStream))
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
			toolStream := make(chan providertypes.ToolEvent, 16)
			m.pendingToolMessageIndex = -1
			m.receivedLiveToolEvents = false
			return m, tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, m.oneShotInput, toolStream), waitToolEventCmd(toolStream))
		}

		if m.mode == modeInteractive {
			return m, textinput.Blink
		}

		return m, nil
	case tea.KeyMsg:
		switch typed.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "ctrl+t":
			if m.mode == modeInteractive && !m.booting {
				m.showTools = !m.showTools
				m.refreshViewport(false)
				return m, nil
			}
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
			m.pendingToolMessageIndex = -1
			m.receivedLiveToolEvents = false
			m.refreshViewport(true)
			toolStream := make(chan providertypes.ToolEvent, 16)
			return m, tea.Batch(m.spinner.Tick, sendPromptCmd(m.ctx, m.promptFn, prompt, toolStream), waitToolEventCmd(toolStream))
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
			if !m.receivedLiveToolEvents && len(typed.result.Metadata.ToolEvents) > 0 {
				for _, block := range mergeToolEvents(typed.result.Metadata.ToolEvents) {
					m.messages = append(m.messages, chatMessage{role: "tool", content: block})
				}
			}
			m.pendingToolMessageIndex = -1
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
	case toolEventMsg:
		m.receivedLiveToolEvents = true
		m.appendOrMergeToolEvent(typed.event)
		m.refreshViewport(false)
		return m, waitToolEventCmd(typed.stream)
	case toolEventStreamClosedMsg:
		return m, nil
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

	toolToggleLabel := "showing"
	if !m.showTools {
		toolToggleLabel = "hidden"
	}
	status := m.theme.status.Render(fmt.Sprintf("💡 Enter send  ·  PgUp/PgDn scroll  ·  End jump latest  ·  Ctrl+T tools:%s  ·  🛑 Ctrl+C/Esc quit", toolToggleLabel))
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

// refreshViewport rebuilds transcript cards and keeps scroll position stable.
func (m *model) refreshViewport(forceBottom bool) {
	previousOffset := m.viewport.YOffset
	var sections []string
	for _, item := range m.messages {
		if item.role == "tool" && !m.showTools {
			continue
		}

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
		case "tool":
			sections = append(sections, m.renderCard(
				m.theme.toolTitle.Render("▛▚ [ 🔧 TOOL ] ▞▜"),
				m.theme.toolBox.Width(m.viewport.Width).Render(strings.TrimSpace(item.content)),
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

// bootView renders the startup animation before interactive input is enabled.
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

// handleViewportKey applies scroll/navigation shortcuts and follow mode updates.
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

// sendPromptCmd wraps prompt execution as an async Bubble Tea command.
func sendPromptCmd(ctx context.Context, promptFn PromptFunc, prompt string, toolStream chan providertypes.ToolEvent) tea.Cmd {
	return func() tea.Msg {
		callCtx := ctx
		if toolStream != nil {
			callCtx = providertypes.WithToolEventHandler(ctx, func(event providertypes.ToolEvent) {
				select {
				case toolStream <- event:
				default:
				}
			})
		}

		result, err := promptFn(callCtx, prompt)
		if toolStream != nil {
			close(toolStream)
		}
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

func formatToolEvent(event providertypes.ToolEvent) string {
	kind := strings.TrimSpace(strings.ToUpper(event.Kind))
	if kind == "" {
		kind = "EVENT"
	}
	tool := strings.TrimSpace(event.Tool)
	if tool == "" {
		tool = "unknown"
	}
	payload := strings.TrimSpace(event.Payload)
	if payload == "" {
		payload = "(no payload)"
	}
	duration := ""
	if strings.EqualFold(kind, "RESULT") {
		duration = fmt.Sprintf("\nduration: %dms", event.DurationMs)
	}

	return fmt.Sprintf("%s: %s\n%s%s", kind, tool, payload, duration)
}

func (m *model) appendOrMergeToolEvent(event providertypes.ToolEvent) {
	kind := strings.TrimSpace(strings.ToLower(event.Kind))
	formatted := formatToolEvent(event)

	if kind == "call" {
		m.messages = append(m.messages, chatMessage{role: "tool", content: formatted})
		m.pendingToolMessageIndex = len(m.messages) - 1
		return
	}

	if kind == "result" && m.pendingToolMessageIndex >= 0 && m.pendingToolMessageIndex < len(m.messages) {
		pending := &m.messages[m.pendingToolMessageIndex]
		if pending.role == "tool" {
			pending.content = strings.TrimSpace(pending.content) + "\n\n" + formatted
			m.pendingToolMessageIndex = -1
			return
		}
	}

	m.messages = append(m.messages, chatMessage{role: "tool", content: formatted})
	m.pendingToolMessageIndex = -1
}

func waitToolEventCmd(stream <-chan providertypes.ToolEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return toolEventStreamClosedMsg{}
		}

		return toolEventMsg{event: event, stream: stream}
	}
}

func mergeToolEvents(events []providertypes.ToolEvent) []string {
	if len(events) == 0 {
		return nil
	}

	blocks := make([]string, 0, len(events))
	lastCallIndex := -1
	for _, event := range events {
		kind := strings.TrimSpace(strings.ToLower(event.Kind))
		switch kind {
		case "call":
			blocks = append(blocks, formatToolEvent(event))
			lastCallIndex = len(blocks) - 1
		case "result":
			resultText := formatToolEvent(event)
			if lastCallIndex >= 0 {
				blocks[lastCallIndex] = blocks[lastCallIndex] + "\n\n" + resultText
				lastCallIndex = -1
				continue
			}
			blocks = append(blocks, resultText)
		default:
			blocks = append(blocks, formatToolEvent(event))
			lastCallIndex = -1
		}
	}

	return blocks
}
