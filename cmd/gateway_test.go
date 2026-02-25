package cmd

import (
	"context"
	"testing"

	channelpkg "miniclaw/pkg/channel"
	"miniclaw/pkg/config"
)

type testAdapter struct{ name string }

func (a testAdapter) Name() string { return a.name }

func (a testAdapter) Run(_ context.Context, _ channelpkg.Handler) error { return nil }

func TestEnabledAdaptersRequiresAtLeastOneChannel(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	if _, err := enabledAdapters(cfg, nil); err == nil {
		t.Fatal("expected error when no channels are enabled")
	}
}

func TestEnabledChannelNames(t *testing.T) {
	t.Parallel()

	adapters := []channelpkg.Adapter{testAdapter{name: "telegram"}, testAdapter{name: "slack"}}
	if got := enabledChannelNames(adapters); got != "telegram,slack" {
		t.Fatalf("enabledChannelNames = %q, want %q", got, "telegram,slack")
	}
}
