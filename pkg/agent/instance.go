package agent

import (
	"context"
	"errors"
	"strings"
	"sync"

	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
)

type Instance struct {
	client    provider.Client
	model     string
	heartbeat config.HeartbeatConfig
	memory    *Memory
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
	response string
	err      error
}

func New(client provider.Client, model string, heartbeat config.HeartbeatConfig) *Instance {
	return &Instance{
		client:    client,
		model:     strings.TrimSpace(model),
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

func (i *Instance) Prompt(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt cannot be empty")
	}

	sessionID := i.SessionID()
	if sessionID == "" {
		return "", errors.New("session is not started")
	}

	response, err := i.client.Prompt(ctx, sessionID, prompt, i.model, "")
	if err != nil {
		return "", err
	}

	i.memory.Append("user", prompt)
	i.memory.Append("assistant", response)

	return response, nil
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

func (i *Instance) EnqueueAndWait(ctx context.Context, prompt string) (string, error) {
	resultCh := make(chan promptResult, 1)
	if err := i.enqueuePrompt(prompt, resultCh); err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultCh:
		return result.response, result.err
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
