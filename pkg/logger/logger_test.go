package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"miniclaw/pkg/config"
)

func TestLoggerJSONEntryShape(t *testing.T) {
	unsetLoggingEnv(t)

	var out bytes.Buffer
	log, err := newWithWriter(config.LoggingConfig{Format: "json", Level: "info"}, &out)
	if err != nil {
		t.Fatalf("newWithWriter error: %v", err)
	}

	log.With("component", "cmd.agent").Info("Prompt event", "request_id", "42", "ok", true)

	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("expected log output")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal log entry: %v", err)
	}

	if entry.Level != "info" {
		t.Fatalf("level = %q, want %q", entry.Level, "info")
	}
	if entry.Message != "Prompt event" {
		t.Fatalf("message = %q, want %q", entry.Message, "Prompt event")
	}
	if entry.Component != "cmd.agent" {
		t.Fatalf("component = %q, want %q", entry.Component, "cmd.agent")
	}
	if entry.Timestamp == "" {
		t.Fatal("expected timestamp")
	}
	if got := entry.Fields["request_id"]; got != "42" {
		t.Fatalf("fields.request_id = %v, want %q", got, "42")
	}
	if got := entry.Fields["ok"]; got != true {
		t.Fatalf("fields.ok = %v, want true", got)
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	unsetLoggingEnv(t)

	var out bytes.Buffer
	log, err := newWithWriter(config.LoggingConfig{Format: "json", Level: "error"}, &out)
	if err != nil {
		t.Fatalf("newWithWriter error: %v", err)
	}

	log.Info("Ignored")
	if got := strings.TrimSpace(out.String()); got != "" {
		t.Fatalf("expected no output for info, got %q", got)
	}

	log.Error("Kept")
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Fatal("expected output for error")
	}
}

func TestLoggerEnvironmentOverrides(t *testing.T) {
	t.Setenv("MINICLAW_LOG_LEVEL", "debug")
	t.Setenv("MINICLAW_LOG_FORMAT", "text")
	defer unsetLoggingEnv(t)

	var out bytes.Buffer
	log, err := newWithWriter(config.LoggingConfig{Format: "json", Level: "error"}, &out)
	if err != nil {
		t.Fatalf("newWithWriter error: %v", err)
	}

	log.Debug("Debug enabled", "component", "test")
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("expected debug output with env override")
	}
	if strings.HasPrefix(line, "{") {
		t.Fatalf("expected text format override, got %q", line)
	}
}

func TestLoggerDefaultsToTextFormat(t *testing.T) {
	unsetLoggingEnv(t)

	var out bytes.Buffer
	log, err := newWithWriter(config.LoggingConfig{}, &out)
	if err != nil {
		t.Fatalf("newWithWriter error: %v", err)
	}

	log.Info("Default format")
	line := strings.TrimSpace(out.String())
	if line == "" {
		t.Fatal("expected log output")
	}
	if strings.HasPrefix(line, "{") {
		t.Fatalf("expected text format by default, got %q", line)
	}
}

func unsetLoggingEnv(t *testing.T) {
	t.Helper()
	_ = os.Unsetenv("MINICLAW_LOG_LEVEL")
	_ = os.Unsetenv("MINICLAW_LOG_FORMAT")
	_ = os.Unsetenv("MINICLAW_LOG_ADD_SOURCE")
}
