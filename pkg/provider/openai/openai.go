package openai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"miniclaw/pkg/config"

	osdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/conversations"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

type Client struct {
	client         osdk.Client
	requestTimeout time.Duration
}

func New(cfg *config.Config) (*Client, error) {
	providerCfg := cfg.Providers.OpenAI
	apiKey := resolveAPIKey(providerCfg)
	if apiKey == "" {
		return nil, errors.New("providers.openai.api_key_env is required or OPENAI_API_KEY must be set")
	}

	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := strings.TrimSpace(providerCfg.BaseURL); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if organization := strings.TrimSpace(providerCfg.Organization); organization != "" {
		opts = append(opts, option.WithOrganization(organization))
	}
	if project := strings.TrimSpace(providerCfg.Project); project != "" {
		opts = append(opts, option.WithProject(project))
	}

	requestTimeout := time.Duration(providerCfg.RequestTimeoutSeconds) * time.Second
	if requestTimeout > 0 {
		opts = append(opts, option.WithRequestTimeout(requestTimeout))
	}

	return &Client{
		client:         osdk.NewClient(opts...),
		requestTimeout: requestTimeout,
	}, nil
}

func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "health")
	startedAt := time.Now()
	log.Debug("provider request started")

	if _, err := c.client.Models.List(ctx); err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return fmt.Errorf("health check failed: %w", err)
	}
	log.Debug("provider request completed", "duration_ms", time.Since(startedAt).Milliseconds())

	return nil
}

func (c *Client) CreateSession(ctx context.Context, title string) (string, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "create_session")
	startedAt := time.Now()
	log.Debug("provider request started", "title_length", len(strings.TrimSpace(title)))

	conversation, err := c.client.Conversations.New(ctx, conversations.ConversationNewParams{})
	if err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return "", fmt.Errorf("create session failed: %w", err)
	}
	if conversation == nil || strings.TrimSpace(conversation.ID) == "" {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", "empty conversation id")
		return "", errors.New("create session returned empty conversation id")
	}
	log.Debug("provider request completed", "duration_ms", time.Since(startedAt).Milliseconds(), "session_id", strings.TrimSpace(conversation.ID))

	return strings.TrimSpace(conversation.ID), nil
}

func (c *Client) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (string, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	log := providerLogger().With("operation", "prompt")
	startedAt := time.Now()

	_ = agent

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session id is required")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}

	normalizedModel, err := normalizeModel(model)
	if err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return "", err
	}
	log.Debug("provider request started",
		"session_id", sessionID,
		"model", normalizedModel,
		"prompt_length", len(prompt),
	)

	response, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
		Model: normalizedModel,
		Input: responses.ResponseNewParamsInputUnion{OfString: osdk.String(prompt)},
		Conversation: responses.ResponseNewParamsConversationUnion{
			OfConversationObject: &responses.ResponseConversationParam{ID: sessionID},
		},
	})
	if err != nil {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	text := strings.TrimSpace(response.OutputText())
	if text == "" {
		log.Debug("provider request failed", "duration_ms", time.Since(startedAt).Milliseconds(), "error", "no output text")
		return "", errors.New("prompt succeeded but returned no text")
	}
	log.Debug("provider request completed", "duration_ms", time.Since(startedAt).Milliseconds(), "response_length", len(text))

	return text, nil
}

func providerLogger() *slog.Logger {
	return slog.Default().With("component", "provider.openai")
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, c.requestTimeout)
}

func resolveAPIKey(cfg config.OpenAIProviderConfig) string {
	if apiKeyEnv := strings.TrimSpace(cfg.APIKeyEnv); apiKeyEnv != "" {
		if apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv)); apiKey != "" {
			return apiKey
		}
	}

	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

func normalizeModel(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", errors.New("model is required")
	}

	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return model, nil
	}

	providerID := strings.TrimSpace(parts[0])
	modelID := strings.TrimSpace(parts[1])
	if providerID == "" || modelID == "" {
		return "", errors.New("model is invalid")
	}
	if providerID != "openai" {
		return "", fmt.Errorf("model provider %q is not supported by openai provider", providerID)
	}

	return modelID, nil
}
