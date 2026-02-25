package bus

import (
	"context"
	"sync"
)

const defaultBufferSize = 100

// MessageBus is an in-process transport for inbound/outbound messages and runtime events.
//
// It is designed for local fan-in/fan-out coordination between runtime components
// without external dependencies.
type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
	handlers map[string]MessageHandler

	eventSubscribers      map[uint64]chan Event
	nextEventSubscriberID uint64

	done      chan struct{}
	closeOnce sync.Once

	mu sync.RWMutex
}

// NewMessageBus creates a message bus with default buffer sizing.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:          make(chan InboundMessage, defaultBufferSize),
		outbound:         make(chan OutboundMessage, defaultBufferSize),
		handlers:         make(map[string]MessageHandler),
		eventSubscribers: make(map[uint64]chan Event),
		done:             make(chan struct{}),
	}
}

// PublishInbound queues one inbound message.
//
// It returns false when the context is canceled or when the bus has been closed.
func (mb *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return false
	case <-mb.done:
		return false
	default:
		// Preflight before send so callers fail fast after bus shutdown.
	}

	select {
	case <-ctx.Done():
		return false
	case <-mb.done:
		return false
	case mb.inbound <- msg:
		return true
	}
}

// ConsumeInbound waits for one inbound message.
//
// It returns false when the context is canceled or when the bus has been closed.
func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return InboundMessage{}, false
	case <-mb.done:
		return InboundMessage{}, false
	case msg := <-mb.inbound:
		return msg, true
	}
}

// PublishOutbound queues one outbound message.
//
// It returns false when the context is canceled or when the bus has been closed.
func (mb *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return false
	case <-mb.done:
		return false
	default:
		// Preflight before send so callers fail fast after bus shutdown.
	}

	select {
	case <-ctx.Done():
		return false
	case <-mb.done:
		return false
	case mb.outbound <- msg:
		return true
	}
}

// SubscribeOutbound waits for one outbound message.
//
// It returns false when the context is canceled or when the bus has been closed.
func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return OutboundMessage{}, false
	case <-mb.done:
		return OutboundMessage{}, false
	case msg := <-mb.outbound:
		return msg, true
	}
}

// RegisterHandler stores a channel handler.
func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

// GetHandler retrieves a previously registered channel handler.
func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

// Close shuts down the bus and closes all event subscriptions.
func (mb *MessageBus) Close() {
	mb.closeOnce.Do(func() {
		close(mb.done)

		mb.mu.Lock()
		for id, ch := range mb.eventSubscribers {
			close(ch)
			delete(mb.eventSubscribers, id)
		}
		mb.mu.Unlock()
	})
}
