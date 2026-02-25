package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"miniclaw/pkg/agent"
	agentprofile "miniclaw/pkg/agent/profile"
	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
	providertypes "miniclaw/pkg/provider/types"
)

// runtimeManager owns per-session agent runtimes for gateway-driven prompts.
type runtimeManager struct {
	ctx    context.Context
	client provider.Client
	cfg    *config.Config
	log    *slog.Logger
	system string

	mu       sync.RWMutex
	runtimes map[string]*sessionRuntime
}

// sessionRuntime is the mutable runtime state tracked for one session key.
type sessionRuntime struct {
	instance   *agent.Instance
	promptMu   sync.Mutex
	cancelLoop context.CancelFunc
}

// newRuntimeManager builds a session runtime manager and resolves the system profile once.
func newRuntimeManager(ctx context.Context, cfg *config.Config, client provider.Client, log *slog.Logger) (*runtimeManager, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	systemProfile, err := agentprofile.ResolveSystemProfile(cfg.Agents.Defaults.Provider)
	if err != nil {
		return nil, fmt.Errorf("resolve agent profile: %w", err)
	}

	if log == nil {
		log = slog.Default()
	}

	return &runtimeManager{
		ctx:      ctx,
		client:   client,
		cfg:      cfg,
		log:      log.With("component", "gateway.runtime_manager"),
		system:   systemProfile,
		runtimes: make(map[string]*sessionRuntime),
	}, nil
}

// Prompt routes one prompt to a session runtime and serializes requests per session.
func (m *runtimeManager) Prompt(ctx context.Context, sessionKey string, prompt string) (providertypes.PromptResult, error) {
	runtime, err := m.runtimeForSession(ctx, sessionKey)
	if err != nil {
		return providertypes.PromptResult{}, err
	}

	runtime.promptMu.Lock()
	defer runtime.promptMu.Unlock()

	if runtime.instance.HeartbeatEnabled() {
		return runtime.instance.EnqueueAndWait(ctx, prompt)
	}

	return runtime.instance.Prompt(ctx, prompt)
}

// runtimeForSession returns an existing runtime or lazily initializes a new one.
func (m *runtimeManager) runtimeForSession(ctx context.Context, sessionKey string) (*sessionRuntime, error) {
	m.mu.RLock()
	runtime, ok := m.runtimes[sessionKey]
	m.mu.RUnlock()
	if ok {
		return runtime, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	runtime, ok = m.runtimes[sessionKey]
	if ok {
		return runtime, nil
	}

	instance := agent.New(m.client, m.cfg.Agents.Defaults.Model, m.cfg.Heartbeat, "", m.system)
	if err := instance.StartSession(ctx, "miniclaw:"+sessionKey); err != nil {
		return nil, fmt.Errorf("start session for %s: %w", sessionKey, err)
	}

	runtime = &sessionRuntime{instance: instance, cancelLoop: func() {}}
	if instance.HeartbeatEnabled() {
		loopCtx, cancelLoop := context.WithCancel(m.ctx)
		runtime.cancelLoop = cancelLoop
		go func() {
			if err := instance.Run(loopCtx); err != nil {
				m.log.Error("Heartbeat loop failed", "session_key", sessionKey, "error", err)
			}
		}()
	}

	m.runtimes[sessionKey] = runtime
	return runtime, nil
}

// Close stops all heartbeat loops and drops tracked session runtimes.
func (m *runtimeManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for sessionKey, runtime := range m.runtimes {
		runtime.cancelLoop()
		delete(m.runtimes, sessionKey)
	}
}
