package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
	providertypes "miniclaw/pkg/provider/types"
	"miniclaw/pkg/ui/chat"

	"github.com/stretchr/testify/require"
)

type fakeProviderClient struct{}

func (f *fakeProviderClient) Health(context.Context) error { return nil }

func (f *fakeProviderClient) CreateSession(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeProviderClient) Prompt(context.Context, string, string, string, string, string) (providertypes.PromptResult, error) {
	return providertypes.PromptResult{}, nil
}

type recordingProviderClient struct {
	mu sync.Mutex

	healthCalls int
	createCalls int
	promptCalls int

	lastSessionID string
	lastPrompt    string
	lastModel     string

	promptText string
	promptErr  error
}

func (c *recordingProviderClient) Health(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCalls++
	return nil
}

func (c *recordingProviderClient) CreateSession(context.Context, string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createCalls++
	return "fantasy-session-test", nil
}

func (c *recordingProviderClient) Prompt(_ context.Context, sessionID string, prompt string, model string, _ string, _ string) (providertypes.PromptResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.promptCalls++
	c.lastSessionID = sessionID
	c.lastPrompt = prompt
	c.lastModel = model
	if c.promptErr != nil {
		return providertypes.PromptResult{}, c.promptErr
	}
	return providertypes.PromptResult{Text: c.promptText}, nil
}

func writeAgentConfig(t *testing.T, cfg config.Config) string {
	t.Helper()

	content, err := json.Marshal(cfg)
	require.NoError(t, err)

	configPath := filepath.Join(t.TempDir(), "config.json")
	err = os.WriteFile(configPath, content, 0o644)
	require.NoError(t, err)

	return configPath
}

func TestResolvePrompt(t *testing.T) {
	original := promptText
	t.Cleanup(func() {
		promptText = original
	})

	promptText = " from-flag "
	if got := resolvePrompt([]string{"from", "args"}); got != "from-flag" {
		t.Fatalf("resolvePrompt with flag = %q, want %q", got, "from-flag")
	}

	promptText = ""
	if got := resolvePrompt([]string{"hello", "world"}); got != "hello world" {
		t.Fatalf("resolvePrompt with args = %q, want %q", got, "hello world")
	}

	if got := resolvePrompt(nil); got != "" {
		t.Fatalf("resolvePrompt without input = %q, want empty", got)
	}
}

func TestResolveAgentType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "defaults to generic", input: "", want: agentTypeGeneric},
		{name: "generic explicit", input: "generic-agent", want: agentTypeGeneric},
		{name: "opencode explicit", input: "opencode-agent", want: agentTypeOpenCode},
		{name: "fantasy explicit", input: "fantasy-agent", want: agentTypeFantasy},
		{name: "trim and lowercase", input: "  OpEnCoDe-AgEnT  ", want: agentTypeOpenCode},
		{name: "trim and lowercase fantasy", input: "  FaNtAsY-AgEnT  ", want: agentTypeFantasy},
		{name: "invalid type", input: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAgentType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveAgentType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveAgentType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRunAgentByTypeRejectsUnsupportedType(t *testing.T) {
	err := runAgentByType("unknown-agent", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported agent type")
	}
}

func TestRunAgentByTypeFantasyUsesInjectedDependencies(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalRunner := runLocalAgentRuntimeWithClientFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runLocalAgentRuntimeWithClientFn = originalRunner
	})

	fakeClient := &fakeProviderClient{}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return fakeClient, nil
	}

	var gotAgentType string
	runLocalAgentRuntimeWithClientFn = func(prompt string, cfg *config.Config, log *slog.Logger, client provider.Client, agentType string) error {
		require.Equal(t, "hello", prompt)
		require.NotNil(t, cfg)
		require.NotNil(t, log)
		require.Same(t, fakeClient, client)
		gotAgentType = agentType
		return nil
	}

	err := runAgentByType(agentTypeFantasy, "hello", &config.Config{}, slog.Default())
	require.NoError(t, err)
	require.Equal(t, agentTypeFantasy, gotAgentType)
}

func TestRunFantasyAgentReturnsFactoryError(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalRunner := runLocalAgentRuntimeWithClientFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runLocalAgentRuntimeWithClientFn = originalRunner
	})

	wantErr := errors.New("factory failed")
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return nil, wantErr
	}
	runLocalAgentRuntimeWithClientFn = func(prompt string, cfg *config.Config, log *slog.Logger, client provider.Client, agentType string) error {
		t.Fatal("runtime should not be called when factory fails")
		return nil
	}

	err := runFantasyAgent("hello", &config.Config{}, slog.Default())
	require.Error(t, err)
	require.ErrorIs(t, err, wantErr)
}

func TestRunFantasyAgentOneShotPromptE2E(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
	})

	client := &recordingProviderClient{promptText: "mock reply"}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	runInteractiveFn = func(context.Context, chat.PromptFunc, chat.RuntimeInfo) {
		t.Fatal("interactive mode should not run for one-shot prompt")
	}

	runSinglePromptFn = func(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
		require.Equal(t, "hello fantasy", prompt)

		result, err := promptFn(ctx, prompt)
		require.NoError(t, err)
		require.Equal(t, "mock reply", result.Text)
	}

	err := runFantasyAgent("hello fantasy", &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
	}, slog.Default())
	require.NoError(t, err)

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Equal(t, 1, client.healthCalls)
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 1, client.promptCalls)
}

func TestAgentCommandOneShotFantasyUsesArgsPromptE2E(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	originalPromptText := promptText
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
		promptText = originalPromptText
	})

	configPath := writeAgentConfig(t, config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Type: agentTypeFantasy, Provider: "openai", Model: "openai/gpt-5.2"}},
	})
	t.Setenv("MINICLAW_CONFIG", configPath)

	client := &recordingProviderClient{promptText: "reply from args"}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	var capturedPrompt string
	runSinglePromptFn = func(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
		capturedPrompt = prompt
		result, err := promptFn(ctx, prompt)
		require.NoError(t, err)
		require.Equal(t, "reply from args", result.Text)
	}
	runInteractiveFn = func(context.Context, chat.PromptFunc, chat.RuntimeInfo) {
		t.Fatal("interactive mode should not run for one-shot prompt")
	}

	promptText = ""
	agentCmd.Run(agentCmd, []string{"prompt from args"})

	require.Equal(t, "prompt from args", capturedPrompt)
}

func TestAgentCommandOneShotFantasyUsesFlagPromptE2E(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	originalPromptText := promptText
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
		promptText = originalPromptText
	})

	configPath := writeAgentConfig(t, config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Type: agentTypeFantasy, Provider: "openai", Model: "openai/gpt-5.2"}},
	})
	t.Setenv("MINICLAW_CONFIG", configPath)

	client := &recordingProviderClient{promptText: "reply from flag"}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	var capturedPrompt string
	runSinglePromptFn = func(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
		capturedPrompt = prompt
		result, err := promptFn(ctx, prompt)
		require.NoError(t, err)
		require.Equal(t, "reply from flag", result.Text)
	}
	runInteractiveFn = func(context.Context, chat.PromptFunc, chat.RuntimeInfo) {
		t.Fatal("interactive mode should not run for one-shot prompt")
	}

	promptText = "  prompt from flag  "
	agentCmd.Run(agentCmd, []string{"ignored args prompt"})

	require.Equal(t, "prompt from flag", capturedPrompt)
}

func TestRunFantasyAgentOneShotPromptE2EProviderError(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
	})

	client := &recordingProviderClient{promptErr: errors.New("prompt failed")}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	runInteractiveFn = func(context.Context, chat.PromptFunc, chat.RuntimeInfo) {
		t.Fatal("interactive mode should not run for one-shot prompt")
	}

	runSinglePromptFn = func(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
		require.Equal(t, "hello failure", prompt)

		_, err := promptFn(ctx, prompt)
		require.Error(t, err)
		require.ErrorContains(t, err, "prompt failed")
	}

	err := runFantasyAgent("hello failure", &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
	}, slog.Default())
	require.NoError(t, err)

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Equal(t, 1, client.healthCalls)
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 1, client.promptCalls)
}

func TestRunFantasyAgentInteractiveRoutesRuntimeInfo(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
	})

	client := &recordingProviderClient{}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	runSinglePromptFn = func(context.Context, chat.PromptFunc, string) {
		t.Fatal("one-shot mode should not run without prompt")
	}

	called := false
	runInteractiveFn = func(_ context.Context, _ chat.PromptFunc, info chat.RuntimeInfo) {
		called = true
		require.Equal(t, agentTypeFantasy, info.AgentType)
		require.Equal(t, "openai", info.Provider)
		require.Equal(t, "openai/gpt-5.2", info.Model)
	}

	err := runFantasyAgent("", &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
	}, slog.Default())
	require.NoError(t, err)
	require.True(t, called)

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Equal(t, 1, client.healthCalls)
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 0, client.promptCalls)
}

func TestRunFantasyAgentOneShotPromptE2EHeartbeatEnabled(t *testing.T) {
	originalFactory := newFantasyProviderClient
	originalSinglePrompt := runSinglePromptFn
	originalInteractive := runInteractiveFn
	t.Cleanup(func() {
		newFantasyProviderClient = originalFactory
		runSinglePromptFn = originalSinglePrompt
		runInteractiveFn = originalInteractive
	})

	client := &recordingProviderClient{promptText: "heartbeat reply"}
	newFantasyProviderClient = func(_ *config.Config) (provider.Client, error) {
		return client, nil
	}

	runInteractiveFn = func(context.Context, chat.PromptFunc, chat.RuntimeInfo) {
		t.Fatal("interactive mode should not run for one-shot prompt")
	}

	runSinglePromptFn = func(ctx context.Context, promptFn chat.PromptFunc, prompt string) {
		require.Equal(t, "hello heartbeat", prompt)

		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		result, err := promptFn(callCtx, prompt)
		require.NoError(t, err)
		require.Equal(t, "heartbeat reply", result.Text)
	}

	err := runFantasyAgent("hello heartbeat", &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
		Heartbeat: config.HeartbeatConfig{
			Enabled:  true,
			Interval: 1,
		},
	}, slog.Default())
	require.NoError(t, err)

	client.mu.Lock()
	defer client.mu.Unlock()
	require.Equal(t, 1, client.healthCalls)
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 1, client.promptCalls)
}

func TestLogStartupConfiguration(t *testing.T) {
	recorder := &recordingHandler{}
	log := slog.New(recorder)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Provider:            "openai",
				Model:               "openai/gpt-5.3",
				Workspace:           "/tmp/workspace",
				RestrictToWorkspace: true,
				MaxTokens:           4096,
				Temperature:         0.2,
				MaxToolIterations:   12,
			},
		},
		Heartbeat: config.HeartbeatConfig{Enabled: true, Interval: 15},
		Logging:   config.LoggingConfig{},
	}

	logStartupConfiguration(log, cfg, "hello")

	if len(recorder.records) != 2 {
		t.Fatalf("records = %d, want 2", len(recorder.records))
	}

	startupRecord := recorder.records[0]
	if startupRecord.Message != "Agent startup" {
		t.Fatalf("startup message = %q, want %q", startupRecord.Message, "Agent startup")
	}
	if got := recordAttrValue(startupRecord, "prompt_mode"); got != "single_prompt" {
		t.Fatalf("prompt_mode = %v, want %q", got, "single_prompt")
	}
	if got := recordAttrValue(startupRecord, "provider"); got != "openai" {
		t.Fatalf("provider = %v, want %q", got, "openai")
	}

	loggingRecord := recorder.records[1]
	if loggingRecord.Message != "Logging configuration" {
		t.Fatalf("logging message = %q, want %q", loggingRecord.Message, "Logging configuration")
	}
	if got := recordAttrValue(loggingRecord, "log_format"); got != "text" {
		t.Fatalf("log_format = %v, want %q", got, "text")
	}
	if got := recordAttrValue(loggingRecord, "log_level"); got != "info" {
		t.Fatalf("log_level = %v, want %q", got, "info")
	}
}

type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(_ string) slog.Handler { return h }

func recordAttrValue(record slog.Record, key string) any {
	var value any
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			value = attr.Value.Any()
			return false
		}
		return true
	})

	return value
}
