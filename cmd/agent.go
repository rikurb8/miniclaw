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

	"miniclaw/pkg/agent"
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

		runtime := agent.New(client, cfg.Agents.Defaults.Model, cfg.Heartbeat)

		ctx := context.Background()
		if err := runtime.StartSession(ctx, "miniclaw"); err != nil {
			fmt.Printf("failed to start session: %v\n", err)
			return
		}

		promptCtx := ctx
		cancelLoop := func() {}
		loopErrCh := make(chan error, 1)
		if runtime.HeartbeatEnabled() {
			promptCtx, cancelLoop = context.WithCancel(ctx)
			go func() {
				loopErrCh <- runtime.Run(promptCtx)
			}()
		}
		defer func() {
			cancelLoop()
			select {
			case loopErr := <-loopErrCh:
				if loopErr != nil {
					fmt.Printf("heartbeat loop failed: %v\n", loopErr)
				}
			default:
			}
		}()

		if prompt != "" {
			runSinglePrompt(promptCtx, runtime, prompt)
			return
		}

		runInteractive(promptCtx, runtime)
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

func runSinglePrompt(ctx context.Context, runtime *agent.Instance, prompt string) {
	response, err := executePrompt(ctx, runtime, prompt)
	if err != nil {
		fmt.Printf("prompt failed: %v\n", err)
		return
	}

	fmt.Println(response)
}

func runInteractive(ctx context.Context, runtime *agent.Instance) {
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

		response, err := executePrompt(ctx, runtime, prompt)
		if err != nil {
			fmt.Printf("prompt failed: %v\n", err)
			continue
		}

		printAssistantMessage(response)
	}
}

func executePrompt(ctx context.Context, runtime *agent.Instance, prompt string) (string, error) {
	if runtime.HeartbeatEnabled() {
		return runtime.EnqueueAndWait(ctx, prompt)
	}

	return runtime.Prompt(ctx, prompt)
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
