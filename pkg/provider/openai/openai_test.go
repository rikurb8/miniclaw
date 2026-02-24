package openai

import (
	"testing"

	"miniclaw/pkg/config"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	cfg := &config.Config{}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
}

func TestNewUsesConfiguredAPIKeyEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("TEST_OPENAI_API_KEY", "sk-test")

	cfg := &config.Config{}
	cfg.Providers.OpenAI.APIKeyEnv = "TEST_OPENAI_API_KEY"

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewFallsBackToDefaultAPIKeyEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-default")
	t.Setenv("TEST_OPENAI_API_KEY", "")

	cfg := &config.Config{}
	cfg.Providers.OpenAI.APIKeyEnv = "TEST_OPENAI_API_KEY"

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain model", input: "gpt-5.2", want: "gpt-5.2"},
		{name: "openai prefix", input: "openai/gpt-5.2", want: "gpt-5.2"},
		{name: "other provider", input: "anthropic/claude", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeModel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeModel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
