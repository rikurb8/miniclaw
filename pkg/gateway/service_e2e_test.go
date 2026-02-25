package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"miniclaw/pkg/bus"
	"miniclaw/pkg/channel"
	"miniclaw/pkg/config"
	providertypes "miniclaw/pkg/provider/types"

	"github.com/stretchr/testify/require"
)

type recordingGatewayProvider struct {
	mu sync.Mutex

	healthCalls       int
	createSessionNext int
	promptSessionIDs  []string
	promptTexts       []string
}

func (p *recordingGatewayProvider) Health(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthCalls++
	return nil
}

func (p *recordingGatewayProvider) CreateSession(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createSessionNext++
	return fmt.Sprintf("session-%d", p.createSessionNext), nil
}

func (p *recordingGatewayProvider) Prompt(_ context.Context, sessionID string, prompt string, _ string, _ string, _ string) (providertypes.PromptResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.promptSessionIDs = append(p.promptSessionIDs, sessionID)
	p.promptTexts = append(p.promptTexts, prompt)
	return providertypes.PromptResult{Text: "ok:" + prompt}, nil
}

func (p *recordingGatewayProvider) snapshot() (int, []string, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sessionIDs := make([]string, len(p.promptSessionIDs))
	copy(sessionIDs, p.promptSessionIDs)

	prompts := make([]string, len(p.promptTexts))
	copy(prompts, p.promptTexts)

	return p.healthCalls, sessionIDs, prompts
}

type scriptedAdapter struct {
	name    string
	inbound []bus.InboundMessage

	continueOnHandlerError bool

	mu       sync.Mutex
	outbound []bus.OutboundMessage
	done     chan struct{}
}

func (a *scriptedAdapter) Name() string {
	return a.name
}

func (a *scriptedAdapter) Run(ctx context.Context, handler channel.Handler) error {
	for _, inbound := range a.inbound {
		outbound, err := handler(ctx, inbound)
		if err != nil && !a.continueOnHandlerError {
			return err
		}

		a.mu.Lock()
		a.outbound = append(a.outbound, outbound)
		a.mu.Unlock()
	}

	close(a.done)

	<-ctx.Done()
	return nil
}

func (a *scriptedAdapter) outbounds() []bus.OutboundMessage {
	a.mu.Lock()
	defer a.mu.Unlock()

	outbound := make([]bus.OutboundMessage, len(a.outbound))
	copy(outbound, a.outbound)
	return outbound
}

func TestGatewayServiceRunE2EFakeAdapterSessionContinuity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := &recordingGatewayProvider{}
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
		Heartbeat: config.HeartbeatConfig{
			Enabled: false,
		},
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: freeTCPPort(t),
		},
	}

	manager, err := newRuntimeManager(ctx, cfg, provider, slog.Default())
	require.NoError(t, err)

	adapter := &scriptedAdapter{
		name: "telegram",
		inbound: []bus.InboundMessage{
			{Channel: "telegram", ChatID: "100", SessionKey: "telegram:100", Content: "one"},
			{Channel: "telegram", ChatID: "100", SessionKey: "telegram:100", Content: "two"},
			{Channel: "telegram", ChatID: "200", SessionKey: "telegram:200", Content: "three"},
		},
		done: make(chan struct{}),
	}

	svc := &Service{
		cfg:      cfg,
		log:      slog.Default().With("component", "gateway.service.test"),
		provider: provider,
		manager:  manager,
		channels: []channel.Adapter{adapter},
		channelStates: map[string]channelState{
			adapter.Name(): {},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	select {
	case <-adapter.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for adapter scripted messages")
	}

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for service run to exit")
	}

	healthCalls, sessionIDs, prompts := provider.snapshot()
	require.GreaterOrEqual(t, healthCalls, 1)
	require.Equal(t, []string{"session-1", "session-1", "session-2"}, sessionIDs)
	require.Equal(t, []string{"one", "two", "three"}, prompts)

	outbounds := adapter.outbounds()
	require.Len(t, outbounds, 3)
	require.Equal(t, "ok:one", outbounds[0].Content)
	require.Equal(t, "ok:two", outbounds[1].Content)
	require.Equal(t, "ok:three", outbounds[2].Content)
	require.Equal(t, "telegram:100", outbounds[0].SessionKey)
	require.Equal(t, "telegram:100", outbounds[1].SessionKey)
	require.Equal(t, "telegram:200", outbounds[2].SessionKey)
}

type failingGatewayProvider struct {
	mu sync.Mutex

	healthCalls int
	promptErr   error
}

type usageGatewayProvider struct{}

func (p *usageGatewayProvider) Health(context.Context) error {
	return nil
}

func (p *usageGatewayProvider) CreateSession(context.Context, string) (string, error) {
	return "session-usage", nil
}

func (p *usageGatewayProvider) Prompt(context.Context, string, string, string, string, string) (providertypes.PromptResult, error) {
	return providertypes.PromptResult{
		Text: "usage-ok",
		Metadata: providertypes.PromptMetadata{
			Usage: &providertypes.TokenUsage{
				InputTokens:         11,
				OutputTokens:        22,
				TotalTokens:         33,
				ReasoningTokens:     4,
				CacheCreationTokens: 5,
				CacheReadTokens:     6,
			},
		},
	}, nil
}

type toggledHealthProvider struct {
	mu sync.Mutex

	healthErr error
}

func (p *toggledHealthProvider) Health(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.healthErr
}

func (p *toggledHealthProvider) CreateSession(context.Context, string) (string, error) {
	return "session-ready", nil
}

func (p *toggledHealthProvider) Prompt(context.Context, string, string, string, string, string) (providertypes.PromptResult, error) {
	return providertypes.PromptResult{Text: "ok"}, nil
}

func (p *toggledHealthProvider) setHealthErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthErr = err
}

func (p *failingGatewayProvider) Health(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthCalls++
	return nil
}

func (p *failingGatewayProvider) CreateSession(context.Context, string) (string, error) {
	return "session-1", nil
}

func (p *failingGatewayProvider) Prompt(context.Context, string, string, string, string, string) (providertypes.PromptResult, error) {
	if p.promptErr != nil {
		return providertypes.PromptResult{}, p.promptErr
	}
	return providertypes.PromptResult{Text: "ok"}, nil
}

func TestGatewayServiceRunE2EProviderPromptFailureReturnsOutboundError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := &failingGatewayProvider{promptErr: fmt.Errorf("prompt exploded")}
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
		Heartbeat: config.HeartbeatConfig{
			Enabled: false,
		},
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: freeTCPPort(t),
		},
	}

	manager, err := newRuntimeManager(ctx, cfg, provider, slog.Default())
	require.NoError(t, err)

	adapter := &scriptedAdapter{
		name:                   "telegram",
		continueOnHandlerError: true,
		inbound: []bus.InboundMessage{
			{Channel: "telegram", ChatID: "100", SessionKey: "telegram:100", Content: "trigger error"},
		},
		done: make(chan struct{}),
	}

	svc := &Service{
		cfg:      cfg,
		log:      slog.Default().With("component", "gateway.service.test"),
		provider: provider,
		manager:  manager,
		channels: []channel.Adapter{adapter},
		channelStates: map[string]channelState{
			adapter.Name(): {},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	select {
	case <-adapter.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for adapter scripted messages")
	}

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for service run to exit")
	}

	outbounds := adapter.outbounds()
	require.Len(t, outbounds, 1)
	require.Equal(t, "", outbounds[0].Content)
	require.Contains(t, outbounds[0].Error, "prompt exploded")
	require.Equal(t, "telegram:100", outbounds[0].SessionKey)
}

func TestGatewayServiceRunE2EUsageMetadataPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := &usageGatewayProvider{}
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
		Heartbeat: config.HeartbeatConfig{
			Enabled: false,
		},
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: freeTCPPort(t),
		},
	}

	manager, err := newRuntimeManager(ctx, cfg, provider, slog.Default())
	require.NoError(t, err)

	adapter := &scriptedAdapter{
		name: "telegram",
		inbound: []bus.InboundMessage{
			{Channel: "telegram", ChatID: "100", SessionKey: "telegram:100", Content: "usage please"},
		},
		done: make(chan struct{}),
	}

	svc := &Service{
		cfg:      cfg,
		log:      slog.Default().With("component", "gateway.service.test"),
		provider: provider,
		manager:  manager,
		channels: []channel.Adapter{adapter},
		channelStates: map[string]channelState{
			adapter.Name(): {},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	select {
	case <-adapter.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for adapter scripted messages")
	}

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for service run to exit")
	}

	outbounds := adapter.outbounds()
	require.Len(t, outbounds, 1)
	require.Equal(t, "usage-ok", outbounds[0].Content)
	require.Equal(t, "11", outbounds[0].Metadata["usage_input_tokens"])
	require.Equal(t, "22", outbounds[0].Metadata["usage_output_tokens"])
	require.Equal(t, "33", outbounds[0].Metadata["usage_total_tokens"])
	require.Equal(t, "4", outbounds[0].Metadata["usage_reasoning_tokens"])
	require.Equal(t, "5", outbounds[0].Metadata["usage_cache_creation_tokens"])
	require.Equal(t, "6", outbounds[0].Metadata["usage_cache_read_tokens"])
}

func TestGatewayServiceReadyzTransitionsOnProviderHealthRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := &toggledHealthProvider{}
	port := freeTCPPort(t)
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{Provider: "openai", Model: "openai/gpt-5.2"}},
		Heartbeat: config.HeartbeatConfig{
			Enabled: false,
		},
		Gateway: config.GatewayConfig{
			Host: "127.0.0.1",
			Port: port,
		},
	}

	manager, err := newRuntimeManager(ctx, cfg, provider, slog.Default())
	require.NoError(t, err)

	adapter := &scriptedAdapter{
		name:    "telegram",
		done:    make(chan struct{}),
		inbound: nil,
	}

	svc := &Service{
		cfg:      cfg,
		log:      slog.Default().With("component", "gateway.service.test"),
		provider: provider,
		manager:  manager,
		channels: []channel.Adapter{adapter},
		channelStates: map[string]channelState{
			adapter.Name(): {},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(ctx)
	}()

	readyURL := fmt.Sprintf("http://127.0.0.1:%d/readyz", port)
	require.Equal(t, http.StatusOK, waitHTTPStatus(t, readyURL, 2*time.Second))

	provider.setHealthErr(fmt.Errorf("temporary provider outage"))
	err = svc.checkProviderHealth(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusServiceUnavailable, waitHTTPStatus(t, readyURL, 2*time.Second))

	provider.setHealthErr(nil)
	err = svc.checkProviderHealth(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, waitHTTPStatus(t, readyURL, 2*time.Second))

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for service run to exit")
	}
}

func waitHTTPStatus(t *testing.T, url string, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		response, err := http.Get(url)
		if err == nil {
			statusCode := response.StatusCode
			require.NoError(t, response.Body.Close())
			return statusCode
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s: %v", url, err)
		}

		time.Sleep(25 * time.Millisecond)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.Port
}
