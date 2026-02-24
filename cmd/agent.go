/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"strings"

	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"

	"github.com/spf13/cobra"
)

var promptText string

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent [prompt]",
	Short: "Send a prompt through the configured provider",
	Long:  "Loads MiniClaw configuration, connects to the configured provider, and sends one prompt.",
	Run: func(cmd *cobra.Command, args []string) {
		prompt := resolvePrompt(args)
		if prompt == "" {
			fmt.Println("prompt is required (use --prompt or pass text as an argument)")
			return
		}

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

		response, err := client.Prompt(ctx, sessionID, prompt, cfg.Agents.Defaults.Model, "")
		if err != nil {
			fmt.Printf("prompt failed: %v\n", err)
			return
		}

		fmt.Println(response)
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
