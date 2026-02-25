package runtime

import (
	"context"
	"log/slog"

	"miniclaw/pkg/bus"
)

func observeAgentEvents(ctx context.Context, messageBus *bus.MessageBus) {
	// Subscribe to a buffered event stream so runtime workers never block on
	// logging. Slow consumers may drop events by design in the bus layer.
	log := slog.Default().With("component", "bus.events")
	events, unsubscribe := messageBus.SubscribeEvents(ctx, 32)
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			// Context cancellation means the owning runtime is shutting down.
			return
		case event, ok := <-events:
			if !ok {
				// Channel closes when unsubscribed or when the bus itself is closed.
				return
			}
			logEvent(log, event)
		}
	}
}

func logEvent(log *slog.Logger, event bus.Event) {
	// Keep a stable attribute set across event types so logs are easy to grep
	// and correlate by request/session identifiers.
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

	// Map operational outcomes to log levels:
	// failures are errors, expected lifecycle milestones are info, and any
	// unknown future event types fall back to debug for safety.
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
