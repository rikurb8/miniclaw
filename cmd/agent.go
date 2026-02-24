/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
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

	"github.com/spf13/cobra"
)

var promptText string
var promptRequestCounter atomic.Uint64

const (
	cliChannelName    = "cli"
	cliChatID         = "local"
	cliSessionKey     = "local"
	agentTypeGeneric  = "generic-agent"
	agentTypeOpenCode = "opencode-agent"
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

		if err := runAgentByType(agentType, prompt, cfg, log); err != nil {
			log.Error("agent runtime failed", "error", err)
		}
	},
}

func runAgentByType(agentType string, prompt string, cfg *config.Config, log *slog.Logger) error {
	switch agentType {
	case agentTypeGeneric:
		return runGenericAgent(prompt, cfg, log)
	case agentTypeOpenCode:
		return runOpenCodeAgent(prompt, cfg, log)
	default:
		return fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

func runGenericAgent(prompt string, cfg *config.Config, log *slog.Logger) error {
	return runLocalAgentRuntime(prompt, cfg, log)
}

func runOpenCodeAgent(prompt string, cfg *config.Config, log *slog.Logger) error {
	return runLocalAgentRuntime(prompt, cfg, log)
}

func runLocalAgentRuntime(prompt string, cfg *config.Config, log *slog.Logger) error {
	client, err := provider.New(cfg)
	if err != nil {
		return fmt.Errorf("initialize provider: %w", err)
	}

	runtime := agent.New(client, cfg.Agents.Defaults.Model, cfg.Heartbeat)

	ctx := context.Background()
	if err := runtime.StartSession(ctx, "miniclaw"); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	log.Info("session started")

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
				log.Error("heartbeat loop failed", "error", loopErr)
			}
		default:
		}
	}()

	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	workerCtx, cancelWorker := context.WithCancel(promptCtx)
	defer cancelWorker()
	go runAgentBusWorker(workerCtx, runtime, messageBus)
	go observeAgentEvents(workerCtx, messageBus)

	if prompt != "" {
		runSinglePrompt(promptCtx, messageBus, prompt)
		return nil
	}

	runInteractive(promptCtx, messageBus)
	return nil
}

func resolveAgentType(input string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return agentTypeGeneric, nil
	}

	switch value {
	case agentTypeGeneric, agentTypeOpenCode:
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
	response, err := executePromptViaBus(ctx, messageBus, prompt)
	if err != nil {
		agentComponentLogger().Error("prompt failed", "error", err)
		return
	}

	fmt.Println(response)
}

func runInteractive(ctx context.Context, messageBus *bus.MessageBus) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("👨🏻 ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				agentComponentLogger().Error("interactive input error", "error", err)
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

		response, err := executePromptViaBus(ctx, messageBus, prompt)
		if err != nil {
			agentComponentLogger().Error("prompt failed", "error", err)
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

func runAgentBusWorker(ctx context.Context, runtime *agent.Instance, messageBus *bus.MessageBus) {
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

		response, err := executePrompt(ctx, runtime, inbound.Content)
		outbound := bus.OutboundMessage{
			Channel:    inbound.Channel,
			ChatID:     inbound.ChatID,
			SessionKey: inbound.SessionKey,
			Content:    response,
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
			_ = messageBus.PublishEvent(ctx, bus.Event{
				Type:       bus.EventPromptCompleted,
				Channel:    inbound.Channel,
				ChatID:     inbound.ChatID,
				SessionKey: inbound.SessionKey,
				RequestID:  requestID,
				Payload: map[string]string{
					"response_length": strconv.Itoa(len(response)),
				},
			})
		}

		if ok := messageBus.PublishOutbound(ctx, outbound); !ok {
			return
		}
	}
}

func executePromptViaBus(ctx context.Context, messageBus *bus.MessageBus, prompt string) (string, error) {
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
			return "", err
		}
		return "", errors.New("unable to enqueue prompt")
	}

	outbound, ok := messageBus.SubscribeOutbound(ctx)
	if !ok {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", errors.New("unable to receive prompt result")
	}

	if outbound.Error != "" {
		return "", errors.New(outbound.Error)
	}

	return outbound.Content, nil
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
		log.Error("prompt event", append(attrs, "error", event.Error)...)
	case bus.EventPromptReceived:
		log.Info("prompt event", attrs...)
	case bus.EventPromptCompleted:
		log.Info("prompt event", attrs...)
	default:
		log.Debug("prompt event", attrs...)
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
