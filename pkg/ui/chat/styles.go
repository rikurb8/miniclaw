package chat

import "github.com/charmbracelet/lipgloss"

// theme groups reusable styles for chat UI regions.
type theme struct {
	header         lipgloss.Style
	headerMeta     lipgloss.Style
	divider        lipgloss.Style
	bootLine       lipgloss.Style
	bootDone       lipgloss.Style
	userBox        lipgloss.Style
	userTitle      lipgloss.Style
	assistantBox   lipgloss.Style
	assistantTitle lipgloss.Style
	toolBox        lipgloss.Style
	toolTitle      lipgloss.Style
	errorBox       lipgloss.Style
	errorTitle     lipgloss.Style
	status         lipgloss.Style
	statusBusy     lipgloss.Style
	statusErr      lipgloss.Style
	hint           lipgloss.Style
	inputLabel     lipgloss.Style
	input          lipgloss.Style
	viewport       lipgloss.Style
}

// defaultTheme defines the retro terminal visual palette used by chat UI.
func defaultTheme() theme {
	return theme{
		header: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("88")),
		headerMeta: lipgloss.NewStyle().
			Foreground(lipgloss.Color("223")),
		divider: lipgloss.NewStyle().
			Foreground(lipgloss.Color("130")),
		bootLine: lipgloss.NewStyle().
			Foreground(lipgloss.Color("180")),
		bootDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("114")).
			Bold(true),
		userBox: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("214")).
			Background(lipgloss.Color("235")).
			Padding(0, 1),
		userTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("214")).
			Padding(0, 1),
		assistantBox: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("44")).
			Background(lipgloss.Color("234")).
			Padding(0, 1),
		assistantTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("44")).
			Padding(0, 1),
		toolBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("109")).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),
		toolTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(lipgloss.Color("109")).
			Padding(0, 1),
		errorBox: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("203")).
			Foreground(lipgloss.Color("203")).
			Background(lipgloss.Color("52")).
			Padding(0, 1),
		errorTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("160")).
			Padding(0, 1),
		status: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Bold(true),
		statusBusy: lipgloss.NewStyle().
			Foreground(lipgloss.Color("222")).
			Bold(true),
		statusErr: lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true),
		hint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")),
		inputLabel: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229")),
		input: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("173")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),
		viewport: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("130")).
			Background(lipgloss.Color("233")).
			Padding(0, 1),
	}
}
