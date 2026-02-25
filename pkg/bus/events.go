package bus

import (
	"context"
	"sync"
	"time"
)

type EventType string

const (
	// EventPromptReceived is emitted when a prompt enters the runtime flow.
	EventPromptReceived EventType = "prompt_received"
	// EventPromptCompleted is emitted when prompt execution completes successfully.
	EventPromptCompleted EventType = "prompt_completed"
	// EventPromptFailed is emitted when prompt execution ends with an error.
	EventPromptFailed EventType = "prompt_failed"
)

// Event is a lightweight runtime signal broadcast to subscribers.
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

// PublishEvent broadcasts one event to all current subscribers.
//
// Delivery is best effort: slow subscribers may miss events instead of blocking publishers.
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

// SubscribeEvents registers a buffered event subscription.
//
// It returns the subscription channel and an idempotent unsubscribe function.
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
