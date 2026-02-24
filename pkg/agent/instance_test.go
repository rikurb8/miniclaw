package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"miniclaw/pkg/config"
)

type fakeProviderClient struct {
	mu sync.Mutex

	healthErr error

	createSessionID string
	createErr       error

	promptResponse string
	promptErr      error

	healthCalls int
	createCalls int
	promptCalls int

	lastSessionID string
	lastPrompt    string
	lastModel     string
	lastAgent     string
}

func (f *fakeProviderClient) Health(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.healthCalls++
	return f.healthErr
}

func (f *fakeProviderClient) CreateSession(ctx context.Context, title string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createCalls++
	if f.createErr != nil {
		return "", f.createErr
	}
	return f.createSessionID, nil
}

func (f *fakeProviderClient) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.promptCalls++
	f.lastSessionID = sessionID
	f.lastPrompt = prompt
	f.lastModel = model
	f.lastAgent = agent

	if f.promptErr != nil {
		return "", f.promptErr
	}
	return f.promptResponse, nil
}

func (f *fakeProviderClient) promptCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.promptCalls
}

func TestStartSession(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{})

	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	if got := inst.SessionID(); got != "session-1" {
		t.Fatalf("SessionID = %q, want %q", got, "session-1")
	}
}

func TestPromptStoresMemory(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "hello back"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{})

	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	response, err := inst.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if response != "hello back" {
		t.Fatalf("response = %q, want %q", response, "hello back")
	}

	entries := inst.MemorySnapshot()
	if len(entries) != 2 {
		t.Fatalf("len(memory) = %d, want 2", len(entries))
	}
	if entries[0].Role != "user" || entries[0].Content != "hello" {
		t.Fatalf("first entry = %#v", entries[0])
	}
	if entries[1].Role != "assistant" || entries[1].Content != "hello back" {
		t.Fatalf("second entry = %#v", entries[1])
	}
}

func TestPromptWithoutSession(t *testing.T) {
	client := &fakeProviderClient{}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{})

	_, err := inst.Prompt(context.Background(), "hello")
	if err == nil {
		t.Fatalf("expected error when session is not started")
	}
}

func TestStartSessionFailsOnHealthError(t *testing.T) {
	client := &fakeProviderClient{healthErr: errors.New("unhealthy")}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{})

	err := inst.StartSession(context.Background(), "miniclaw")
	if err == nil {
		t.Fatalf("expected health error")
	}
}

func TestEnqueueAndWaitRejectsEmptyPrompt(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1"}
	inst := New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1})
	if err := inst.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	_, err := inst.EnqueueAndWait(context.Background(), "   ")
	if err == nil {
		t.Fatalf("expected error for empty prompt")
	}
}
