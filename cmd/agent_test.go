package cmd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"miniclaw/pkg/agent"
	"miniclaw/pkg/bus"
	"miniclaw/pkg/config"
)

func TestIsExitCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "exit", want: true},
		{input: " quit ", want: true},
		{input: ":q", want: true},
		{input: "EXIT", want: true},
		{input: "hello", want: false},
		{input: "quit now", want: false},
	}

	for _, tt := range tests {
		if got := isExitCommand(tt.input); got != tt.want {
			t.Fatalf("isExitCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAssistantLines(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOut []string
	}{
		{name: "single line", input: "hello", wantOut: []string{"hello"}},
		{name: "multi line", input: "one\ntwo", wantOut: []string{"one", "two"}},
		{name: "trim outer whitespace", input: "  one\ntwo  ", wantOut: []string{"one", "two"}},
		{name: "empty input", input: "   ", wantOut: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assistantLines(tt.input)
			if !reflect.DeepEqual(got, tt.wantOut) {
				t.Fatalf("assistantLines(%q) = %#v, want %#v", tt.input, got, tt.wantOut)
			}
		})
	}
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

func TestPrintAssistantMessage(t *testing.T) {
	output := captureStdout(t, func() {
		printAssistantMessage("first\nsecond")
	})

	if output != "🦞 first\n🦞 second\n\n" {
		t.Fatalf("printAssistantMessage output = %q", output)
	}

	emptyOutput := captureStdout(t, func() {
		printAssistantMessage("   ")
	})
	if emptyOutput != "" {
		t.Fatalf("expected no output for empty message, got %q", emptyOutput)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stdout = w

	outCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		var builder strings.Builder
		_, copyErr := io.Copy(&builder, r)
		if copyErr != nil {
			errCh <- copyErr
			return
		}
		outCh <- builder.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = original

	select {
	case copyErr := <-errCh:
		_ = r.Close()
		t.Fatalf("read captured stdout: %v", copyErr)
	case output := <-outCh:
		_ = r.Close()
		return output
	}

	return ""
}

type fakeCmdProviderClient struct {
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

func (f *fakeCmdProviderClient) Health(ctx context.Context) error {
	return f.healthErr
}

func (f *fakeCmdProviderClient) CreateSession(ctx context.Context, title string) (string, error) {
	return f.createSessionID, nil
}

func (f *fakeCmdProviderClient) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (string, error) {
	f.promptCallCount++
	f.lastSessionID = sessionID
	f.lastPrompt = prompt
	f.lastModel = model
	f.lastAgent = agent
	if f.promptErr != nil {
		return "", f.promptErr
	}
	return f.promptResponse, nil
}

func TestExecutePromptHeartbeatDisabledUsesDirectPrompt(t *testing.T) {
	client := &fakeCmdProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: false})
	if err := runtime.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	got, err := executePrompt(context.Background(), runtime, "ping")
	if err != nil {
		t.Fatalf("executePrompt error: %v", err)
	}
	if got != "pong" {
		t.Fatalf("response = %q, want %q", got, "pong")
	}
	if client.promptCallCount != 1 {
		t.Fatalf("prompt calls = %d, want 1", client.promptCallCount)
	}
}

func TestExecutePromptHeartbeatEnabledUsesQueue(t *testing.T) {
	client := &fakeCmdProviderClient{createSessionID: "session-1", promptResponse: "pong"}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: true, Interval: 1})
	if err := runtime.StartSession(context.Background(), "miniclaw"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	respCh := make(chan string, 1)
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
			if got != "pong" {
				t.Fatalf("response = %q, want %q", got, "pong")
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
	client := &fakeCmdProviderClient{createSessionID: "session-1", promptErr: wantErr}
	runtime := agent.New(client, "openai/gpt-5.2", config.HeartbeatConfig{Enabled: false})
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
