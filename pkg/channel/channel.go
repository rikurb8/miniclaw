package channel

import (
	"context"

	"miniclaw/pkg/bus"
)

// Handler processes one inbound channel message and returns an outbound reply.
type Handler func(context.Context, bus.InboundMessage) (bus.OutboundMessage, error)

// Adapter bridges one external transport (for example Telegram) into MiniClaw.
type Adapter interface {
	Name() string
	Run(context.Context, Handler) error
}
