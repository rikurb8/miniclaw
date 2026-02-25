package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"miniclaw/pkg/agent"
	"miniclaw/pkg/bus"
	"miniclaw/pkg/config"
	providertypes "miniclaw/pkg/provider/types"
)

type fakeProviderClient struct {
	healthErr       error
	createSessionID string
	promptResponse  string
	promptErr       error
	promptCallCount int
	lastPrompt      string
	lastSessionID   string
	lastModel       string
	lastAgent       string
}

func (f *fakeProviderClient) Health(ctx context.Context) error {
	return f.healthErr
}

func (f *fakeProviderClient) CreateSession(ctx context.Context, title string) (string, error) {
	return f.createSessionID, nil
}

func (f *fakeProviderClient) Prompt(ctx context.Context, sessionID string, prompt string, model string, agentName string, systemPrompt string) (providertypes.PromptResult, error) {
	_ = systemPrompt

	f.promptCallCount++
	f.lastSessionID = sessionID
	f.lastPrompt = prompt
	f.lastModel = model
	f.lastAgent = agentName
	if f.promptErr != nil {
		return providertypes.PromptResult{}, f.promptErr
	}
	return providertypes.PromptResult{Text: f.promptResponse}, nil
}

func TestExecutePromptHeartbeatDisabledUsesDirectPrompt(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: false}, "", "")
	if err := runtime.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	got, err := executePrompt(context.Background(), runtime, "ping")
	if err != nil {
		t.Fatalf("executePrompt error: %v", err)
	}
	if got.Text != "pong" {
		t.Fatalf("response = %q, want %q", got.Text, "pong")
	}
	if client.promptCallCount != 1 {
		t.Fatalf("prompt calls = %d, want 1", client.promptCallCount)
	}
}

func TestExecutePromptHeartbeatEnabledUsesQueue(t *testing.T) {
	client := &fakeProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1}, "", "")
	if err := runtime.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	respCh := make(chan providertypes.PromptResult, 1)
	errCh := make(chan error, 1)
	go func() {
		response, err := executePrompt(context.Background(), runtime, "ping")
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
			t.Fatalf("executePrompt error: %v", err)
		case got := <-respCh:
			if got.Text != "pong" {
				t.Fatalf("response = %q, want %q", got.Text, "pong")
			}
			if client.promptCallCount != 1 {
				t.Fatalf("prompt calls = %d, want 1", client.promptCallCount)
			}
			return
		default:
		}

		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for heartbeat prompt execution")
		}

		if err := runtime.Step(context.Background()); err != nil {
			t.Fatalf("Step error: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestExecutePromptPropagatesError(t *testing.T) {
	wantErr := errors.New("prompt failed")
	client := &fakeProviderClient{createSessionID: "session-1", promptErr: wantErr}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: false}, "", "")
	if err := runtime.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	_, err := executePrompt(context.Background(), runtime, "ping")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestLogEventLevels(t *testing.T) {
	recorder := &recordingHandler{}
	log := slog.New(recorder)

	logEvent(log, bus.Event{Type: bus.EventPromptReceived, RequestID: "1"})
	if got := recorder.LastLevel(); got != slog.LevelInfo {
		t.Fatalf("received event level = %v, want %v", got, slog.LevelInfo)
	}

	logEvent(log, bus.Event{Type: bus.EventPromptCompleted, RequestID: "2"})
	if got := recorder.LastLevel(); got != slog.LevelInfo {
		t.Fatalf("completed event level = %v, want %v", got, slog.LevelInfo)
	}

	logEvent(log, bus.Event{Type: bus.EventPromptFailed, RequestID: "3", Error: "boom"})
	if got := recorder.LastLevel(); got != slog.LevelError {
		t.Fatalf("failed event level = %v, want %v", got, slog.LevelError)
	}
}

func TestPromptResultMetadataIncludesUsage(t *testing.T) {
	metadata := PromptResultMetadata(providertypes.PromptResult{
		Text: "hello",
		Metadata: providertypes.PromptMetadata{
			Usage: &providertypes.TokenUsage{
				InputTokens:         10,
				OutputTokens:        20,
				TotalTokens:         30,
				ReasoningTokens:     5,
				CacheCreationTokens: 2,
				CacheReadTokens:     7,
			},
		},
	})

	if got := metadata[UsageInputTokensKey]; got != "10" {
		t.Fatalf("input usage = %q, want %q", got, "10")
	}
	if got := metadata[UsageOutputTokensKey]; got != "20" {
		t.Fatalf("output usage = %q, want %q", got, "20")
	}
	if got := metadata[UsageTotalTokensKey]; got != "30" {
		t.Fatalf("total usage = %q, want %q", got, "30")
	}
}

func TestPromptResultFromOutboundParsesUsage(t *testing.T) {
	result := PromptResultFromOutbound(bus.OutboundMessage{
		Content: "answer",
		Metadata: map[string]string{
			UsageInputTokensKey:       "11",
			UsageOutputTokensKey:      "22",
			UsageTotalTokensKey:       "33",
			UsageReasoningTokensKey:   "4",
			UsageCacheCreateTokensKey: "5",
			UsageCacheReadTokensKey:   "6",
		},
	})

	if result.Metadata.Usage == nil {
		t.Fatal("expected usage metadata")
	}
	if result.Metadata.Usage.TotalTokens != 33 {
		t.Fatalf("total tokens = %d, want 33", result.Metadata.Usage.TotalTokens)
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

func (h *recordingHandler) LastLevel() slog.Level {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return 0
	}
	return h.records[len(h.records)-1].Level
}
