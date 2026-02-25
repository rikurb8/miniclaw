package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"

	"miniclaw/pkg/agent"
	agentprofile "miniclaw/pkg/agent/profile"
	"miniclaw/pkg/bus"
	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
	providertypes "miniclaw/pkg/provider/types"
)

const (
	cliChannelName = "cli"
	cliChatID      = "local"
	cliSessionKey  = "local"
)

// LocalSession coordinates a single local CLI session.
//
// It owns:
//   - one agent instance,
//   - one in-process message bus,
//   - one bus worker goroutine,
//   - and (optionally) one heartbeat loop goroutine.
//
// Prompt requests are routed through the bus so UI code and runtime execution
// share the same transport semantics.
type LocalSession struct {
	runtime    *agent.Instance
	messageBus *bus.MessageBus
	log        *slog.Logger

	cancelLoop   context.CancelFunc
	loopErrCh    chan error
	cancelWorker context.CancelFunc

	requestCounter atomic.Uint64
}

func StartLocalSession(ctx context.Context, cfg *config.Config, log *slog.Logger, client provider.Client, observeEvents bool) (*LocalSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	if log == nil {
		log = slog.Default()
	}

	systemProfile, err := agentprofile.ResolveSystemProfile(cfg.Agents.Defaults.Provider)
	if err != nil {
		return nil, fmt.Errorf("resolve agent profile: %w", err)
	}

	runtime := agent.New(client, cfg.Agents.Defaults.Model, cfg.Heartbeat, "", systemProfile)
	if err := runtime.StartSession(ctx, "miniclaw"); err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}

	session := &LocalSession{
		runtime:      runtime,
		messageBus:   bus.NewMessageBus(),
		log:          log,
		cancelLoop:   func() {},
		loopErrCh:    make(chan error, 1),
		cancelWorker: func() {},
	}

	workerCtx, cancelWorker := context.WithCancel(ctx)
	session.cancelWorker = cancelWorker
	go runAgentBusWorker(workerCtx, runtime, session.messageBus)

	if runtime.HeartbeatEnabled() {
		loopCtx, cancelLoop := context.WithCancel(ctx)
		session.cancelLoop = cancelLoop
		go func() {
			session.loopErrCh <- runtime.Run(loopCtx)
		}()
	}

	if observeEvents {
		go observeAgentEvents(workerCtx, session.messageBus)
	}

	return session, nil
}

func (s *LocalSession) Prompt(ctx context.Context, prompt string) (providertypes.PromptResult, error) {
	if s == nil {
		return providertypes.PromptResult{}, errors.New("local session is nil")
	}

	return executePromptViaBus(ctx, &s.requestCounter, s.messageBus, prompt)
}

// Close shuts down worker and heartbeat resources owned by the session.
//
// Shutdown is best-effort and non-blocking for heartbeat completion to avoid
// hanging CLI exit if the provider loop is already winding down.
func (s *LocalSession) Close() {
	if s == nil {
		return
	}

	s.cancelWorker()
	s.cancelLoop()
	s.messageBus.Close()

	select {
	case loopErr := <-s.loopErrCh:
		if loopErr != nil {
			s.log.Error("Heartbeat loop failed", "error", loopErr)
		}
	default:
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
			Metadata:   PromptResultMetadata(result),
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

				usagePayload[UsageInputTokensKey] = strconv.FormatInt(usage.InputTokens, 10)
				usagePayload[UsageOutputTokensKey] = strconv.FormatInt(usage.OutputTokens, 10)
				usagePayload[UsageTotalTokensKey] = strconv.FormatInt(usage.TotalTokens, 10)
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

func executePromptViaBus(ctx context.Context, counter *atomic.Uint64, messageBus *bus.MessageBus, prompt string) (providertypes.PromptResult, error) {
	requestID := strconv.FormatUint(counter.Add(1), 10)
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

	return PromptResultFromOutbound(outbound), nil
}
