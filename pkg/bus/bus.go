package bus

import (
	"context"
	"sync"
)

const defaultBufferSize = 100

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

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:          make(chan InboundMessage, defaultBufferSize),
		outbound:         make(chan OutboundMessage, defaultBufferSize),
		handlers:         make(map[string]MessageHandler),
		eventSubscribers: make(map[uint64]chan Event),
		done:             make(chan struct{}),
	}
}

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

func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

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
