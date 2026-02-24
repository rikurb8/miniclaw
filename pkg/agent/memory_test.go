package agent

import (
	"sync"
	"testing"
)

func TestMemoryAppendListClear(t *testing.T) {
	m := NewMemory()
	m.Append("user", "hello")
	m.Append("assistant", "hi")

	entries := m.List()
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Role != "user" || entries[0].Content != "hello" {
		t.Fatalf("first entry = %#v", entries[0])
	}
	if entries[1].Role != "assistant" || entries[1].Content != "hi" {
		t.Fatalf("second entry = %#v", entries[1])
	}

	m.Clear()
	if got := len(m.List()); got != 0 {
		t.Fatalf("len(entries) after clear = %d, want 0", got)
	}
}

func TestMemoryConcurrentAppend(t *testing.T) {
	m := NewMemory()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			m.Append("user", "hello")
		}()
	}

	wg.Wait()

	if got := len(m.List()); got != n {
		t.Fatalf("len(entries) = %d, want %d", got, n)
	}
}
