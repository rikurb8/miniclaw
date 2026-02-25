package opencode

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"miniclaw/pkg/config"
	providertypes "miniclaw/pkg/provider/types"

	sdk "github.com/sst/opencode-sdk-go"
	"github.com/sst/opencode-sdk-go/option"
)

type Client struct {
	client         *sdk.Client
	requestTimeout time.Duration
}

type healthResponse struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

func New(cfg *config.Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.Providers.OpenCode.BaseURL)
	if baseURL == "" {
		return nil, errors.New("providers.opencode.base_url is required")
	}

	opts := []option.RequestOption{option.WithBaseURL(baseURL)}
	if authHeader, ok := buildBasicAuthHeader(cfg.Providers.OpenCode); ok {
		opts = append(opts, option.WithHeader("Authorization", authHeader))
	}

	requestTimeout := time.Duration(cfg.Providers.OpenCode.RequestTimeoutSeconds) * time.Second

	return &Client{
		client:         sdk.NewClient(opts...),
		requestTimeout: requestTimeout,
	}, nil
}

func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "health")
	startedAt := time.Now()
	log.Debug("provider request started")

	var response healthResponse
	if err := c.client.Get(ctx, "/global/health", nil, &response); err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return fmt.Errorf("health check failed: %w", err)
	}
	if !response.Healthy {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", "server unhealthy")
		return errors.New("opencode server reported unhealthy status")
	}
	log.Debug("provider request completed", "duration_ms", time.Since(startedAt).Milliseconds(), "version", response.Version)
	return nil
}

func (c *Client) CreateSession(ctx context.Context, title string) (string, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "create_session")
	startedAt := time.Now()
	log.Debug("provider request started", "title_length", len(strings.TrimSpace(title)))

	params := sdk.SessionNewParams{}
	if strings.TrimSpace(title) != "" {
		params.Title = sdk.F(strings.TrimSpace(title))
	}

	session, err := c.client.Session.New(ctx, params)
	if err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return "", fmt.Errorf("create session failed: %w", err)
	}
	if session.ID == "" {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", "empty session id")
		return "", errors.New("create session returned empty session id")
	}
	log.Debug("provider request completed", "duration_ms", time.Since(startedAt).Milliseconds(), "session_id", session.ID)

	return session.ID, nil
}

func (c *Client) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (providertypes.PromptResult, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "prompt")
	startedAt := time.Now()
	log.Debug("provider request started",
		"session_id", strings.TrimSpace(sessionID),
		"model", strings.TrimSpace(model),
		"agent", strings.TrimSpace(agent),
		"prompt_length", len(strings.TrimSpace(prompt)),
	)

	params := sdk.SessionPromptParams{
		Parts: sdk.F([]sdk.SessionPromptParamsPartUnion{
			sdk.TextPartInputParam{
				Type: sdk.F(sdk.TextPartInputTypeText),
				Text: sdk.F(prompt),
			},
		}),
	}

	if strings.TrimSpace(agent) != "" {
		params.Agent = sdk.F(strings.TrimSpace(agent))
	}

	if providerID, modelID, ok := parseModelRef(model); ok {
		params.Model = sdk.F(sdk.SessionPromptParamsModel{
			ProviderID: sdk.F(providerID),
			ModelID:    sdk.F(modelID),
		})
	}

	response, err := c.client.Session.Prompt(ctx, sessionID, params)
	if err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return providertypes.PromptResult{}, fmt.Errorf("prompt failed: %w", err)
	}

	text := extractText(response.Parts)
	if text == "" {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", "no text parts")
		return providertypes.PromptResult{}, errors.New("prompt succeeded but returned no text parts")
	}
	log.Debug("provider request completed",
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"response_length", len(text),
		"parts_count", len(response.Parts),
	)

	usage := providertypes.TokenUsage{
		InputTokens:     tokenCount(response.Info.Tokens.Input),
		OutputTokens:    tokenCount(response.Info.Tokens.Output),
		TotalTokens:     tokenCount(response.Info.Tokens.Input) + tokenCount(response.Info.Tokens.Output),
		ReasoningTokens: tokenCount(response.Info.Tokens.Reasoning),
		CacheReadTokens: tokenCount(response.Info.Tokens.Cache.Read),
	}
	var usagePtr *providertypes.TokenUsage
	if !usage.IsZero() {
		usagePtr = &usage
	}

	return providertypes.PromptResult{
		Text: text,
		Metadata: providertypes.PromptMetadata{
			Provider: strings.TrimSpace(response.Info.ProviderID),
			Model:    strings.TrimSpace(response.Info.ModelID),
			Agent:    strings.TrimSpace(agent),
			Usage:    usagePtr,
		},
	}, nil
}

func providerLogger() *slog.Logger {
	return slog.Default().With("component", "provider.opencode")
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

func buildBasicAuthHeader(cfg config.OpenCodeProviderConfig) (string, bool) {
	passwordEnv := strings.TrimSpace(cfg.PasswordEnv)
	if passwordEnv == "" {
		return "", false
	}

	password := strings.TrimSpace(os.Getenv(passwordEnv))
	if password == "" {
		return "", false
	}

	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		username = "opencode"
	}

	token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return "Basic " + token, true
}

func parseModelRef(input string) (providerID string, modelID string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(input), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	providerID = strings.TrimSpace(parts[0])
	modelID = strings.TrimSpace(parts[1])
	if providerID == "" || modelID == "" {
		return "", "", false
	}

	return providerID, modelID, true
}

func extractText(parts []sdk.Part) string {
	var lines []string
	for _, part := range parts {
		if part.Type == sdk.PartTypeText {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				lines = append(lines, text)
			}
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func tokenCount(value float64) int64 {
	if value <= 0 {
		return 0
	}

	return int64(math.Round(value))
}
