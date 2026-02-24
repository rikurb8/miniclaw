package provider

import (
	"testing"

	"miniclaw/pkg/config"
	provideropencode "miniclaw/pkg/provider/opencode"
)

func TestNewDefaultsToOpenCodeProvider(t *testing.T) {
	cfg := &config.Config{}
	cfg.Providers.OpenCode.BaseURL = "http://127.0.0.1:4096"

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := client.(*provideropencode.Client); !ok {
		t.Fatalf("expected *opencode.Client, got %T", client)
	}
}

func TestNewReturnsErrorForUnsupportedProvider(t *testing.T) {
	cfg := &config.Config{}
	cfg.Agents.Defaults.Provider = "unknown"

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}
