package agent

import (
	"context"
	"errors"
	"strings"
	"sync"

	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
	providertypes "miniclaw/pkg/provider/types"
)

type Instance struct {
	client    provider.Client
	model     string
	agent     string
	system    string
	heartbeat config.HeartbeatConfig
	memory    *Memory
	// queueWake is a coalescing signal channel: one token means "queue has work".
	queueWake chan struct{}

	mu        sync.RWMutex
	sessionID string
	queue     []queuedPrompt
}

type queuedPrompt struct {
	prompt   string
	resultCh chan promptResult
}

type promptResult struct {
	result providertypes.PromptResult
	err    error
}

func New(client provider.Client, model string, heartbeat config.HeartbeatConfig, agent string, system string) *Instance {
	return &Instance{
		client:    client,
		model:     strings.TrimSpace(model),
		agent:     strings.TrimSpace(agent),
		system:    strings.TrimSpace(system),
		heartbeat: heartbeat,
		memory:    NewMemory(),
		queueWake: make(chan struct{}, 1),
	}
}

func (i *Instance) StartSession(ctx context.Context, title string) error {
	if err := i.client.Health(ctx); err != nil {
		return err
	}

	sessionID, err := i.client.CreateSession(ctx, title)
	if err != nil {
		return err
	}

	i.mu.Lock()
	i.sessionID = sessionID
	i.mu.Unlock()

	return nil
}

func (i *Instance) Prompt(ctx context.Context, prompt string) (providertypes.PromptResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return providertypes.PromptResult{}, errors.New("prompt cannot be empty")
	}

	sessionID := i.SessionID()
	if sessionID == "" {
		return providertypes.PromptResult{}, errors.New("session is not started")
	}

	result, err := i.client.Prompt(ctx, sessionID, prompt, i.model, i.agent, i.system)
	if err != nil {
		return providertypes.PromptResult{}, err
	}

	i.memory.Append("user", prompt)
	i.memory.Append("assistant", result.Text)

	return result, nil
}

func (i *Instance) SessionID() string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.sessionID
}

func (i *Instance) MemorySnapshot() []MemoryEntry {
	return i.memory.List()
}

func (i *Instance) HeartbeatEnabled() bool {
	return i.heartbeat.Enabled
}

func (i *Instance) EnqueuePrompt(prompt string) {
	i.enqueuePrompt(prompt, nil)
}

func (i *Instance) EnqueueAndWait(ctx context.Context, prompt string) (providertypes.PromptResult, error) {
	resultCh := make(chan promptResult, 1)
	if err := i.enqueuePrompt(prompt, resultCh); err != nil {
		return providertypes.PromptResult{}, err
	}

	select {
	case <-ctx.Done():
		return providertypes.PromptResult{}, ctx.Err()
	case result := <-resultCh:
		return result.result, result.err
	}
}

func (i *Instance) enqueuePrompt(prompt string, resultCh chan promptResult) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return errors.New("prompt cannot be empty")
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	i.queue = append(i.queue, queuedPrompt{prompt: prompt, resultCh: resultCh})
	if i.heartbeat.Enabled {
		select {
		case i.queueWake <- struct{}{}:
		default:
			// A wake signal is already pending; the queued item will be processed soon.
		}
	}
	return nil
}

func (i *Instance) dequeuePrompt() (queuedPrompt, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.queue) == 0 {
		return queuedPrompt{}, false
	}

	item := i.queue[0]
	i.queue = i.queue[1:]
	return item, true
}

func (i *Instance) queueWakeChannel() <-chan struct{} {
	return i.queueWake
}
