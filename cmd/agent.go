/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	agentruntime "miniclaw/pkg/agent/runtime"
	"miniclaw/pkg/config"
	"miniclaw/pkg/logger"
	"miniclaw/pkg/provider"
	providerfantasy "miniclaw/pkg/provider/fantasy"
	"miniclaw/pkg/ui/chat"

	"github.com/spf13/cobra"
)

var promptText string

const (
	agentTypeGeneric  = "generic-agent"
	agentTypeOpenCode = "opencode-agent"
	agentTypeFantasy  = "fantasy-agent"
)

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

		agentType, err := resolveAgentType(cfg.Agents.Defaults.Type)
		if err != nil {
			fmt.Printf("failed to resolve agent type: %v\n", err)
			return
		}

		appLogger, err := logger.New(cfg.Logging)
		if err != nil {
			fmt.Printf("failed to initialize logger: %v\n", err)
			return
		}
		slog.SetDefault(appLogger)
		log := agentComponentLogger().With("agent_type", agentType)
		if shouldShowRuntimeLogs(cfg.Logging.Level) {
			logStartupConfiguration(log, cfg, prompt)
		}

		if err := runAgentByType(agentType, prompt, cfg, log); err != nil {
			log.Error("Agent runtime failed", "error", err)
		}
	},
}

func runAgentByType(agentType string, prompt string, cfg *config.Config, log *slog.Logger) error {
	switch agentType {
	case agentTypeGeneric:
		return runGenericAgent(prompt, cfg, log)
	case agentTypeOpenCode:
		return runOpenCodeAgent(prompt, cfg, log)
	case agentTypeFantasy:
		return runFantasyAgent(prompt, cfg, log)
	default:
		return fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

func runGenericAgent(prompt string, cfg *config.Config, log *slog.Logger) error {
	return runLocalAgentRuntime(prompt, cfg, log, agentTypeGeneric)
}

func runOpenCodeAgent(prompt string, cfg *config.Config, log *slog.Logger) error {
	return runLocalAgentRuntime(prompt, cfg, log, agentTypeOpenCode)
}

func runFantasyAgent(prompt string, cfg *config.Config, log *slog.Logger) error {
	client, err := providerfantasy.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize fantasy provider: %w", err)
	}

	return runLocalAgentRuntimeWithClient(prompt, cfg, log, client, agentTypeFantasy)
}

func runLocalAgentRuntime(prompt string, cfg *config.Config, log *slog.Logger, agentType string) error {
	client, err := provider.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize provider: %w", err)
	}

	return runLocalAgentRuntimeWithClient(prompt, cfg, log, client, agentType)
}

func runLocalAgentRuntimeWithClient(prompt string, cfg *config.Config, log *slog.Logger, client provider.Client, agentType string) error {
	ctx := context.Background()
	session, err := agentruntime.StartLocalSession(ctx, cfg, log, client, shouldShowRuntimeLogs(cfg.Logging.Level))
	if err != nil {
		return err
	}
	defer session.Close()

	if shouldShowRuntimeLogs(cfg.Logging.Level) {
		log.Info("Session started")
	}

	if prompt != "" {
		runSinglePrompt(ctx, session.Prompt, prompt)
		return nil
	}

	runInteractive(ctx, session.Prompt, chat.RuntimeInfo{
		AgentType: agentType,
		Provider:  strings.TrimSpace(cfg.Agents.Defaults.Provider),
		Model:     strings.TrimSpace(cfg.Agents.Defaults.Model),
	})
	return nil
}

func logStartupConfiguration(log *slog.Logger, cfg *config.Config, prompt string) {
	promptMode := "interactive"
	if strings.TrimSpace(prompt) != "" {
		promptMode = "single_prompt"
	}

	log.Info("Agent startup",
		"prompt_mode", promptMode,
		"provider", cfg.Agents.Defaults.Provider,
		"model", cfg.Agents.Defaults.Model,
		"workspace", cfg.Agents.Defaults.Workspace,
		"restrict_to_workspace", cfg.Agents.Defaults.RestrictToWorkspace,
		"max_tokens", cfg.Agents.Defaults.MaxTokens,
		"temperature", cfg.Agents.Defaults.Temperature,
		"max_tool_iterations", cfg.Agents.Defaults.MaxToolIterations,
		"heartbeat_enabled", cfg.Heartbeat.Enabled,
		"heartbeat_interval_seconds", cfg.Heartbeat.Interval,
	)

	log.Info("Logging configuration",
		"log_format", defaultString(cfg.Logging.Format, "text"),
		"log_level", defaultString(cfg.Logging.Level, "info"),
		"log_add_source", cfg.Logging.AddSource,
	)
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return strings.ToLower(trimmed)
}

func resolveAgentType(input string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return agentTypeGeneric, nil
	}

	switch value {
	case agentTypeGeneric, agentTypeOpenCode, agentTypeFantasy:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported agent type: %s", input)
	}
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

func runSinglePrompt(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
	if err := chat.RunOneShot(ctx, promptFn, prompt); err != nil {
		agentComponentLogger().Error("One-shot UI failed", "error", err)
	}
}

func runInteractive(ctx context.Context, promptFn chat.PromptFunc, info chat.RuntimeInfo) {
	if err := chat.RunInteractive(ctx, promptFn, info); err != nil {
		agentComponentLogger().Error("Interactive UI failed", "error", err)
	}
}

func agentComponentLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.agent")
}

func shouldShowRuntimeLogs(level string) bool {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(os.Getenv("MINICLAW_LOG_LEVEL")))
	}

	return value == "debug"
}
