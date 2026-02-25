package bus

import (
	"context"
	"sync"
	"time"
)

type EventType string

const (
	EventPromptReceived  EventType = "prompt_received"
	EventPromptCompleted EventType = "prompt_completed"
	EventPromptFailed    EventType = "prompt_failed"
)

type Event struct {
	Type       EventType         `json:"type"`
	At         time.Time         `json:"at"`
	Channel    string            `json:"channel,omitempty"`
	ChatID     string            `json:"chat_id,omitempty"`
	SessionKey string            `json:"session_key,omitempty"`
	RequestID  string            `json:"request_id,omitempty"`
	Payload    map[string]string `json:"payload,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func (mb *MessageBus) PublishEvent(ctx context.Context, event Event) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	select {
	case <-ctx.Done():
		return false
	case <-mb.done:
		return false
	default:
	}

	mb.mu.RLock()
	subs := make([]chan Event, 0, len(mb.eventSubscribers))
	for _, ch := range mb.eventSubscribers {
		subs = append(subs, ch)
	}
	mb.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop instead of blocking the publisher on slow subscribers.
		}
	}

	return true
}

func (mb *MessageBus) SubscribeEvents(ctx context.Context, buffer int) (<-chan Event, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if buffer <= 0 {
		buffer = defaultBufferSize
	}

	ch := make(chan Event, buffer)

	mb.mu.Lock()
	select {
	case <-mb.done:
		mb.mu.Unlock()
		close(ch)
		return ch, func() {}
	default:
	}

	id := mb.nextEventSubscriberID
	mb.nextEventSubscriberID++
	mb.eventSubscribers[id] = ch
	mb.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			mb.mu.Lock()
			if eventCh, ok := mb.eventSubscribers[id]; ok {
				delete(mb.eventSubscribers, id)
				close(eventCh)
			}
			mb.mu.Unlock()
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			unsubscribe()
		case <-mb.done:
			unsubscribe()
		}
	}()

	return ch, unsubscribe
}
