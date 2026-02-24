package agent

import (
	"strings"
	"sync"
	"time"
)

type MemoryEntry struct {
	Role    string
	Content string
	At      time.Time
}

type Memory struct {
	mu      sync.RWMutex
	entries []MemoryEntry
}

func NewMemory() *Memory {
	return &Memory{}
}

func (m *Memory) Append(role string, content string) {
	role = strings.TrimSpace(role)
	content = strings.TrimSpace(content)
	if role == "" || content == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = append(m.entries, MemoryEntry{
		Role:    role,
		Content: content,
		At:      time.Now().UTC(),
	})
}

func (m *Memory) List() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.entries) == 0 {
		return nil
	}

	out := make([]MemoryEntry, len(m.entries))
	copy(out, m.entries)
	return out
}

func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = nil
}
