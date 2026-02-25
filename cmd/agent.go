/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"miniclaw/pkg/agent"
	"miniclaw/pkg/bus"
	"miniclaw/pkg/config"
	"miniclaw/pkg/logger"
	"miniclaw/pkg/provider"
	providerfantasy "miniclaw/pkg/provider/fantasy"
	providertypes "miniclaw/pkg/provider/types"
	"miniclaw/pkg/ui/chat"

	"github.com/spf13/cobra"
)

var promptText string
var promptRequestCounter atomic.Uint64

const (
	cliChannelName          = "cli"
	cliChatID               = "local"
	cliSessionKey           = "local"
	agentTypeGeneric        = "generic-agent"
	agentTypeOpenCode       = "opencode-agent"
	agentTypeFantasy        = "fantasy-agent"
	metaUsageInKey          = "usage_input_tokens"
	metaUsageOutKey         = "usage_output_tokens"
	metaUsageTotalKey       = "usage_total_tokens"
	metaUsageReasonKey      = "usage_reasoning_tokens"
	metaUsageCacheCreateKey = "usage_cache_creation_tokens"
	metaUsageCacheReadKey   = "usage_cache_read_tokens"
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
	runtime := agent.New(client, cfg.Agents.Defaults.Model, cfg.Heartbeat)

	ctx := context.Background()
	if err := runtime.StartSession(ctx, "miniclaw"); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	if shouldShowRuntimeLogs(cfg.Logging.Level) {
		log.Info("Session started")
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
				log.Error("Heartbeat loop failed", "error", loopErr)
			}
		default:
		}
	}()

	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	workerCtx, cancelWorker := context.WithCancel(promptCtx)
	defer cancelWorker()
	go runAgentBusWorker(workerCtx, runtime, messageBus)
	if shouldShowRuntimeLogs(cfg.Logging.Level) {
		go observeAgentEvents(workerCtx, messageBus)
	}

	if prompt != "" {
		runSinglePrompt(promptCtx, messageBus, prompt)
		return nil
	}

	runInteractive(promptCtx, messageBus, chat.RuntimeInfo{
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

func runSinglePrompt(ctx context.Context, messageBus *bus.MessageBus, prompt string) {
	promptFn := func(runCtx context.Context, text string) (providertypes.PromptResult, error) {
		return executePromptViaBus(runCtx, messageBus, text)
	}

	if err := chat.RunOneShot(ctx, promptFn, prompt); err != nil {
		agentComponentLogger().Error("One-shot UI failed", "error", err)
	}
}

func runInteractive(ctx context.Context, messageBus *bus.MessageBus, info chat.RuntimeInfo) {
	promptFn := func(runCtx context.Context, text string) (providertypes.PromptResult, error) {
		return executePromptViaBus(runCtx, messageBus, text)
	}

	if err := chat.RunInteractive(ctx, promptFn, info); err != nil {
		agentComponentLogger().Error("Interactive UI failed", "error", err)
	}
}

func executePrompt(ctx context.Context, runtime *agent.Instance, prompt string) (providertypes.PromptResult, error) {
	if runtime.HeartbeatEnabled() {
		return runtime.EnqueueAndWait(ctx, prompt)
	}

	return runtime.Prompt(ctx, prompt)
}

func runAgentBusWorker(ctx context.Context, runtime *agent.Instance, messageBus *bus.MessageBus) {
	var sessionUsageIn int64
	var sessionUsageOut int64
	var sessionUsageTotal int64

	for {
		inbound, ok := messageBus.ConsumeInbound(ctx)
		if !ok {
			return
		}

		requestID := inbound.Metadata["request_id"]
		_ = messageBus.PublishEvent(ctx, bus.Event{
			Type:       bus.EventPromptReceived,
			Channel:    inbound.Channel,
			ChatID:     inbound.ChatID,
			SessionKey: inbound.SessionKey,
			RequestID:  requestID,
			Payload: map[string]string{
				"prompt_length": strconv.Itoa(len(inbound.Content)),
			},
		})

		result, err := executePrompt(ctx, runtime, inbound.Content)
		outbound := bus.OutboundMessage{
			Channel:    inbound.Channel,
			ChatID:     inbound.ChatID,
			SessionKey: inbound.SessionKey,
			Content:    result.Text,
			Metadata:   promptResultMetadata(result),
		}
		if err != nil {
			outbound.Error = err.Error()
			_ = messageBus.PublishEvent(ctx, bus.Event{
				Type:       bus.EventPromptFailed,
				Channel:    inbound.Channel,
				ChatID:     inbound.ChatID,
				SessionKey: inbound.SessionKey,
				RequestID:  requestID,
				Error:      err.Error(),
			})
		} else {
			usagePayload := map[string]string{
				"response_length": strconv.Itoa(len(result.Text)),
			}
			if result.Metadata.Usage != nil {
				usage := result.Metadata.Usage
				sessionUsageIn += usage.InputTokens
				sessionUsageOut += usage.OutputTokens
				sessionUsageTotal += usage.TotalTokens

				usagePayload[metaUsageInKey] = strconv.FormatInt(usage.InputTokens, 10)
				usagePayload[metaUsageOutKey] = strconv.FormatInt(usage.OutputTokens, 10)
				usagePayload[metaUsageTotalKey] = strconv.FormatInt(usage.TotalTokens, 10)
				usagePayload["session_usage_input_tokens"] = strconv.FormatInt(sessionUsageIn, 10)
				usagePayload["session_usage_output_tokens"] = strconv.FormatInt(sessionUsageOut, 10)
				usagePayload["session_usage_total_tokens"] = strconv.FormatInt(sessionUsageTotal, 10)
			}
			_ = messageBus.PublishEvent(ctx, bus.Event{
				Type:       bus.EventPromptCompleted,
				Channel:    inbound.Channel,
				ChatID:     inbound.ChatID,
				SessionKey: inbound.SessionKey,
				RequestID:  requestID,
				Payload:    usagePayload,
			})
		}

		if ok := messageBus.PublishOutbound(ctx, outbound); !ok {
			return
		}
	}
}

func executePromptViaBus(ctx context.Context, messageBus *bus.MessageBus, prompt string) (providertypes.PromptResult, error) {
	requestID := strconv.FormatUint(promptRequestCounter.Add(1), 10)
	inbound := bus.InboundMessage{
		Channel:    cliChannelName,
		ChatID:     cliChatID,
		SessionKey: cliSessionKey,
		Content:    prompt,
		Metadata: map[string]string{
			"request_id": requestID,
		},
	}

	if ok := messageBus.PublishInbound(ctx, inbound); !ok {
		if err := ctx.Err(); err != nil {
			return providertypes.PromptResult{}, err
		}
		return providertypes.PromptResult{}, errors.New("unable to enqueue prompt")
	}

	outbound, ok := messageBus.SubscribeOutbound(ctx)
	if !ok {
		if err := ctx.Err(); err != nil {
			return providertypes.PromptResult{}, err
		}
		return providertypes.PromptResult{}, errors.New("unable to receive prompt result")
	}

	if outbound.Error != "" {
		return providertypes.PromptResult{}, errors.New(outbound.Error)
	}

	return providerResultFromOutbound(outbound), nil
}

func promptResultMetadata(result providertypes.PromptResult) map[string]string {
	if result.Metadata.Usage == nil {
		return nil
	}

	usage := result.Metadata.Usage
	return map[string]string{
		metaUsageInKey:          strconv.FormatInt(usage.InputTokens, 10),
		metaUsageOutKey:         strconv.FormatInt(usage.OutputTokens, 10),
		metaUsageTotalKey:       strconv.FormatInt(usage.TotalTokens, 10),
		metaUsageReasonKey:      strconv.FormatInt(usage.ReasoningTokens, 10),
		metaUsageCacheCreateKey: strconv.FormatInt(usage.CacheCreationTokens, 10),
		metaUsageCacheReadKey:   strconv.FormatInt(usage.CacheReadTokens, 10),
	}
}

func providerResultFromOutbound(outbound bus.OutboundMessage) providertypes.PromptResult {
	result := providertypes.PromptResult{Text: outbound.Content}
	if outbound.Metadata == nil {
		return result
	}

	usage := &providertypes.TokenUsage{
		InputTokens:         parseInt64(outbound.Metadata[metaUsageInKey]),
		OutputTokens:        parseInt64(outbound.Metadata[metaUsageOutKey]),
		TotalTokens:         parseInt64(outbound.Metadata[metaUsageTotalKey]),
		ReasoningTokens:     parseInt64(outbound.Metadata[metaUsageReasonKey]),
		CacheCreationTokens: parseInt64(outbound.Metadata[metaUsageCacheCreateKey]),
		CacheReadTokens:     parseInt64(outbound.Metadata[metaUsageCacheReadKey]),
	}

	if usage.IsZero() {
		return result
	}

	result.Metadata.Usage = usage
	return result
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}

	return parsed
}

func observeAgentEvents(ctx context.Context, messageBus *bus.MessageBus) {
	log := slog.Default().With("component", "bus.events")
	events, unsubscribe := messageBus.SubscribeEvents(ctx, 32)
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			logEvent(log, event)
		}
	}
}

func agentComponentLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.agent")
}

func logEvent(log *slog.Logger, event bus.Event) {
	attrs := []any{
		"event_type", event.Type,
		"request_id", event.RequestID,
		"channel", event.Channel,
		"chat_id", event.ChatID,
		"session_key", event.SessionKey,
		"timestamp", event.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if len(event.Payload) > 0 {
		attrs = append(attrs, "payload", event.Payload)
	}

	switch event.Type {
	case bus.EventPromptFailed:
		log.Error("Prompt event", append(attrs, "error", event.Error)...)
	case bus.EventPromptReceived:
		log.Info("Prompt event", attrs...)
	case bus.EventPromptCompleted:
		log.Info("Prompt event", attrs...)
	default:
		log.Debug("Prompt event", attrs...)
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

func shouldShowRuntimeLogs(level string) bool {
	value := strings.ToLower(strings.TrimSpace(level))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(os.Getenv("MINICLAW_LOG_LEVEL")))
	}

	return value == "debug"
}
