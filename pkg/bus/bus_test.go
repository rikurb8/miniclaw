package bus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInboundRoundTrip(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	in := InboundMessage{Channel: "cli", Content: "hello", SessionKey: "session-1"}
	if ok := mb.PublishInbound(context.Background(), in); !ok {
		t.Fatal("expected inbound publish to succeed")
	}

	out, ok := mb.ConsumeInbound(context.Background())
	if !ok {
		t.Fatal("expected inbound consume to succeed")
	}
	if out.Content != in.Content {
		t.Fatalf("content = %q, want %q", out.Content, in.Content)
	}
}

func TestOutboundRoundTrip(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	in := OutboundMessage{Channel: "cli", Content: "world", SessionKey: "session-1"}
	if ok := mb.PublishOutbound(context.Background(), in); !ok {
		t.Fatal("expected outbound publish to succeed")
	}

	out, ok := mb.SubscribeOutbound(context.Background())
	if !ok {
		t.Fatal("expected outbound subscribe to succeed")
	}
	if out.Content != in.Content {
		t.Fatalf("content = %q, want %q", out.Content, in.Content)
	}
}

func TestCloseStopsBusOperations(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()

	if ok := mb.PublishInbound(context.Background(), InboundMessage{Content: "hello"}); ok {
		t.Fatal("expected inbound publish to fail after close")
	}
	if ok := mb.PublishOutbound(context.Background(), OutboundMessage{Content: "hello"}); ok {
		t.Fatal("expected outbound publish to fail after close")
	}

	if _, ok := mb.ConsumeInbound(context.Background()); ok {
		t.Fatal("expected inbound consume to stop after close")
	}
	if _, ok := mb.SubscribeOutbound(context.Background()); ok {
		t.Fatal("expected outbound subscribe to stop after close")
	}
}

func TestContextCancellation(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ok := mb.PublishInbound(ctx, InboundMessage{Content: "hello"}); ok {
		t.Fatal("expected publish to fail on canceled context")
	}

	if _, ok := mb.ConsumeInbound(ctx); ok {
		t.Fatal("expected consume to fail on canceled context")
	}
}

func TestRegisterAndGetHandler(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	h := func(InboundMessage) error { return nil }
	mb.RegisterHandler("cli", h)

	got, ok := mb.GetHandler("cli")
	if !ok {
		t.Fatal("expected handler")
	}
	if got == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestConsumeUnblocksOnClose(t *testing.T) {
	mb := NewMessageBus()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = mb.ConsumeInbound(context.Background())
	}()

	mb.Close()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("consume did not unblock after close")
	}
}

func TestSubscribeUnblocksOnClose(t *testing.T) {
	mb := NewMessageBus()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = mb.SubscribeOutbound(context.Background())
	}()

	mb.Close()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subscribe did not unblock after close")
	}
}

func TestHandlerErrorShape(t *testing.T) {
	wantErr := errors.New("boom")
	h := func(InboundMessage) error { return wantErr }

	if err := h(InboundMessage{}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestEventFanout(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	ctx := context.Background()
	eventsA, unsubA := mb.SubscribeEvents(ctx, 1)
	defer unsubA()
	eventsB, unsubB := mb.SubscribeEvents(ctx, 1)
	defer unsubB()

	event := Event{Type: EventPromptReceived, RequestID: "1"}
	if ok := mb.PublishEvent(ctx, event); !ok {
		t.Fatal("expected event publish to succeed")
	}

	select {
	case got := <-eventsA:
		if got.Type != EventPromptReceived {
			t.Fatalf("event type = %q, want %q", got.Type, EventPromptReceived)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subscriber A did not receive event")
	}

	select {
	case got := <-eventsB:
		if got.Type != EventPromptReceived {
			t.Fatalf("event type = %q, want %q", got.Type, EventPromptReceived)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subscriber B did not receive event")
	}
}

func TestSlowSubscriberDoesNotBlockPublishEvent(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	ctx := context.Background()
	events, unsubscribe := mb.SubscribeEvents(ctx, 1)
	defer unsubscribe()

	if ok := mb.PublishEvent(ctx, Event{Type: EventPromptReceived}); !ok {
		t.Fatal("expected first event publish to succeed")
	}

	start := time.Now()
	if ok := mb.PublishEvent(ctx, Event{Type: EventPromptCompleted}); !ok {
		t.Fatal("expected second event publish to succeed")
	}

	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("publish event blocked on slow subscriber")
	}

	select {
	case <-events:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected at least one event")
	}
}

func TestUnsubscribeStopsEvents(t *testing.T) {
	mb := NewMessageBus()
	t.Cleanup(mb.Close)

	ctx := context.Background()
	events, unsubscribe := mb.SubscribeEvents(ctx, 1)
	unsubscribe()

	if ok := mb.PublishEvent(ctx, Event{Type: EventPromptReceived}); !ok {
		t.Fatal("expected event publish to succeed")
	}

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected closed event channel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected event channel close after unsubscribe")
	}
}

func TestSubscribeEventsUnblocksOnClose(t *testing.T) {
	mb := NewMessageBus()

	ctx := context.Background()
	events, _ := mb.SubscribeEvents(ctx, 1)
	mb.Close()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected event channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("event subscription did not unblock after close")
	}
}
