package chat

import (
	"context"
	"fmt"

	providertypes "miniclaw/pkg/provider/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PromptFunc func(ctx context.Context, prompt string) (providertypes.PromptResult, error)

type RuntimeInfo struct {
	AgentType string
	Provider  string
	Model     string
}

func RunInteractive(ctx context.Context, promptFn PromptFunc, info RuntimeInfo) error {
	model := newModel(ctx, promptFn, modeInteractive, "", info)
	program := tea.NewProgram(model)
	_, err := program.Run()
	if err != nil {
		return err
	}

	fmt.Print("\033[H\033[2J")
	fmt.Println(renderGoodbyeBanner())
	return nil
}

func RunOneShot(ctx context.Context, promptFn PromptFunc, prompt string) error {
	model := newModel(ctx, promptFn, modeOneShot, prompt, RuntimeInfo{})
	program := tea.NewProgram(model)
	_, err := program.Run()
	return err
}

func renderGoodbyeBanner() string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("88")).
		Padding(1, 2)

	return style.Render("🦞 Thanks for using MiniClaw")
}
