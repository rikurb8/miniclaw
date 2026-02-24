package provider

import (
	"context"
	"fmt"

	"miniclaw/pkg/config"
	provideropenai "miniclaw/pkg/provider/openai"
	"miniclaw/pkg/provider/opencode"
)

type Client interface {
	Health(ctx context.Context) error
	CreateSession(ctx context.Context, title string) (string, error)
	Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (string, error)
}

func New(cfg *config.Config) (Client, error) {
	providerID := cfg.Agents.Defaults.Provider
	if providerID == "" {
		providerID = "opencode"
	}

	switch providerID {
	case "opencode":
		return opencode.New(cfg)
	case "openai":
		return provideropenai.New(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerID)
	}
}
