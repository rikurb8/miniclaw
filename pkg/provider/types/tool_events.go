package types

import (
	"context"
	"strings"
)

type toolEventHandlerKey struct{}

// ToolEventHandler receives tool events emitted during prompt execution.
type ToolEventHandler func(event ToolEvent)

// WithToolEventHandler returns a context carrying a tool event handler.
func WithToolEventHandler(ctx context.Context, handler ToolEventHandler) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if handler == nil {
		return ctx
	}

	return context.WithValue(ctx, toolEventHandlerKey{}, handler)
}

// HasToolEventHandler reports whether the context carries a handler.
func HasToolEventHandler(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	_, ok := ToolEventHandlerFromContext(ctx)
	return ok
}

// ToolEventHandlerFromContext returns a context-carried tool event handler.
func ToolEventHandlerFromContext(ctx context.Context) (ToolEventHandler, bool) {
	if ctx == nil {
		return nil, false
	}

	handler, ok := ctx.Value(toolEventHandlerKey{}).(ToolEventHandler)
	if !ok || handler == nil {
		return nil, false
	}

	return handler, true
}

// EmitToolEvent emits one normalized tool event to a context handler, when present.
func EmitToolEvent(ctx context.Context, event ToolEvent) {
	if ctx == nil {
		return
	}

	handler, ok := ctx.Value(toolEventHandlerKey{}).(ToolEventHandler)
	if !ok || handler == nil {
		return
	}

	event.Kind = strings.TrimSpace(event.Kind)
	event.Tool = strings.TrimSpace(event.Tool)
	event.Payload = strings.TrimSpace(event.Payload)
	handler(event)
}
