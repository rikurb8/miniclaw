package cmd

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"miniclaw/pkg/config"
)

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
