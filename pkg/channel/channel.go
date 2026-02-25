package channel

import (
	"context"

	"miniclaw/pkg/bus"
)

// Handler processes one inbound channel message and returns an outbound reply.
type Handler func(context.Context, bus.InboundMessage) (bus.OutboundMessage, error)

// Adapter bridges one external transport (for example Telegram) into MiniClaw.
//
// Implementations own transport-specific receive/send mechanics and delegate
// prompt execution to the shared Handler.
type Adapter interface {
	// Name returns the stable channel identifier (for example "telegram").
	Name() string
	// Run starts the adapter loop and blocks until context cancellation or fatal error.
	Run(context.Context, Handler) error
}
