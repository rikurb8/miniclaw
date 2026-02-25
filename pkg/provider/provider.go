package provider

import (
	"context"
	"fmt"
	"log/slog"

	"miniclaw/pkg/config"
	provideropenai "miniclaw/pkg/provider/openai"
	"miniclaw/pkg/provider/opencode"
	providertypes "miniclaw/pkg/provider/types"
)

type Client interface {
	Health(ctx context.Context) error
	CreateSession(ctx context.Context, title string) (string, error)
	Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (providertypes.PromptResult, error)
}

func New(cfg *config.Config) (Client, error) {
	providerID := cfg.Agents.Defaults.Provider
	if providerID == "" {
		providerID = "opencode"
	}

	slog.Default().With("component", "provider.factory").Debug("Resolving provider client", "provider", providerID)

	switch providerID {
	case "opencode":
		return opencode.New(cfg)
	case "openai":
		return provideropenai.New(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerID)
	}
}
