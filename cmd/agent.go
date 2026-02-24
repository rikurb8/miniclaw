/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"

	"github.com/spf13/cobra"
)

var promptText string

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent [prompt]",
	Short: "Send a prompt or start an interactive chat",
	Long:  "Loads MiniClaw configuration, connects to the configured provider, and sends one prompt or starts an interactive chat.",
	Run: func(cmd *cobra.Command, args []string) {
		prompt := resolvePrompt(args)

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("failed to load config: %v\n", err)
			return
		}

		client, err := provider.New(cfg)
		if err != nil {
			fmt.Printf("failed to initialize provider: %v\n", err)
			return
		}

		ctx := context.Background()
		if err := client.Health(ctx); err != nil {
			fmt.Printf("provider health check failed: %v\n", err)
			return
		}

		sessionID, err := client.CreateSession(ctx, "miniclaw")
		if err != nil {
			fmt.Printf("failed to create session: %v\n", err)
			return
		}

		if prompt != "" {
			runSinglePrompt(ctx, client, sessionID, cfg.Agents.Defaults.Model, prompt)
			return
		}

		runInteractive(ctx, client, sessionID, cfg.Agents.Defaults.Model)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&promptText, "prompt", "p", "", "prompt text to send")
}

func resolvePrompt(args []string) string {
	if value := strings.TrimSpace(promptText); value != "" {
		return value
	}

	if len(args) == 0 {
		return ""
	}

	value := strings.TrimSpace(strings.Join(args, " "))
	if value == "" {
		return ""
	}

	return value
}

func runSinglePrompt(ctx context.Context, client provider.Client, sessionID string, model string, prompt string) {
	response, err := client.Prompt(ctx, sessionID, prompt, model, "")
	if err != nil {
		fmt.Printf("prompt failed: %v\n", err)
		return
	}

	fmt.Println(response)
}

func runInteractive(ctx context.Context, client provider.Client, sessionID string, model string) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("👨🏻 ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Printf("input error: %v\n", err)
			}
			return
		}

		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}
		if isExitCommand(prompt) {
			return
		}

		response, err := client.Prompt(ctx, sessionID, prompt, model, "")
		if err != nil {
			fmt.Printf("prompt failed: %v\n", err)
			continue
		}

		printAssistantMessage(response)
	}
}

func printAssistantMessage(message string) {
	lines := assistantLines(message)
	for _, line := range lines {
		fmt.Printf("🦞 %s\n", line)
	}
	if len(lines) > 0 {
		fmt.Println()
	}
}

func assistantLines(message string) []string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func isExitCommand(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "exit", "quit", ":q":
		return true
	default:
		return false
	}
}
