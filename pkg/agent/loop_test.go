package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"miniclaw/pkg/config"
)

func TestRunDisabledHeartbeatReturnsImmediately(t *testing.T) {
	client := &fakeProviderClient{}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: false})

	if err := inst.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
}

func TestStepConsumesQueuedPrompt(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{})

	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	inst.EnqueuePrompt("ping")
	if err := inst.Step(context.Background()); err != nil {
		t.Fatalf("Step error: %v", err)
	}

	if got := client.promptCallCount(); got != 1 {
		t.Fatalf("prompt calls = %d, want 1", got)
	}
}

func TestRunProcessesQueuedPrompt(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1})

	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	inst.EnqueuePrompt("ping")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- inst.Run(ctx)
	}()

	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if client.promptCallCount() > 0 {
			cancel()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	err := <-errCh
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if got := client.promptCallCount(); got != 1 {
		t.Fatalf("prompt calls = %d, want 1", got)
	}

	entries := inst.MemorySnapshot()
	if len(entries) != 2 {
		t.Fatalf("len(memory) = %d, want 2", len(entries))
	}
}

func TestRunWithInvalidIntervalFails(t *testing.T) {
	client := &fakeProviderClient{}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 0})

	err := inst.Run(context.Background())
	if err == nil {
		t.Fatalf("expected invalid interval error")
	}
}

func TestEnqueueAndWaitReturnsStepResult(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1})
	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	respCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		response, err := inst.EnqueueAndWait(context.Background(), "ping")
		if err != nil {
			errCh <- err
			return
		}
		respCh <- response
	}()

	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		select {
		case err := <-errCh:
			t.Fatalf("EnqueueAndWait error: %v", err)
		case response := <-respCh:
			if response != "pong" {
				t.Fatalf("response = %q, want %q", response, "pong")
			}
			return
		default:
		}

		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for queued prompt result")
		}

		if err := inst.Step(context.Background()); err != nil {
			t.Fatalf("Step error: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEnqueueAndWaitRespectsContextCancel(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1})
	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := inst.EnqueueAndWait(ctx, "ping")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
}
