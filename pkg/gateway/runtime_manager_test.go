package gateway

import (
	"context"
	"sync"
	"testing"

	"miniclaw/pkg/config"
	providertypes "miniclaw/pkg/provider/types"
)

type fakeProviderClient struct {
	mu                 sync.Mutex
	createSessionCount int
	promptCount        int
	prompts            []string
}

func (f *fakeProviderClient) Health(context.Context) error {
	return nil
}

func (f *fakeProviderClient) CreateSession(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createSessionCount++
	return "session-id", nil
}

func (f *fakeProviderClient) Prompt(_ context.Context, _ string, prompt string, _ string, _ string, _ string) (providertypes.PromptResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.promptCount++
	f.prompts = append(f.prompts, prompt)
	return providertypes.PromptResult{Text: "ok:" + prompt}, nil
}

func TestRuntimeManagerReusesSessionRuntime(t *testing.T) {
	t.Parallel()

	fakeClient := &fakeProviderClient{}
	cfg := &config.Config{
		Agents:    config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5-nano"}},
		Heartbeat: config.HeartbeatConfig{Enabled: false},
	}

	manager, err := newRuntimeManager(context.Background(), cfg, fakeClient, nil)
	if err != nil {
		t.Fatalf("newRuntimeManager error: %v", err)
	}
	t.Cleanup(manager.Close)

	if _, err := manager.Prompt(context.Background(), "telegram:100", "one"); err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if _, err := manager.Prompt(context.Background(), "telegram:100", "two"); err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	fakeClient.mu.Lock()
	defer fakeClient.mu.Unlock()
	if fakeClient.createSessionCount != 1 {
		t.Fatalf("createSessionCount = %d, want 1", fakeClient.createSessionCount)
	}
	if fakeClient.promptCount != 2 {
		t.Fatalf("promptCount = %d, want 2", fakeClient.promptCount)
	}
}

func TestRuntimeManagerCreatesSessionPerSessionKey(t *testing.T) {
	t.Parallel()

	fakeClient := &fakeProviderClient{}
	cfg := &config.Config{
		Agents:    config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5-nano"}},
		Heartbeat: config.HeartbeatConfig{Enabled: false},
	}

	manager, err := newRuntimeManager(context.Background(), cfg, fakeClient, nil)
	if err != nil {
		t.Fatalf("newRuntimeManager error: %v", err)
	}
	t.Cleanup(manager.Close)

	if _, err := manager.Prompt(context.Background(), "telegram:100", "one"); err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if _, err := manager.Prompt(context.Background(), "telegram:200", "two"); err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	fakeClient.mu.Lock()
	defer fakeClient.mu.Unlock()
	if fakeClient.createSessionCount != 2 {
		t.Fatalf("createSessionCount = %d, want 2", fakeClient.createSessionCount)
	}
}
