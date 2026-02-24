package fantasy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	core "charm.land/fantasy"
	provideropenai "charm.land/fantasy/providers/openai"

	"miniclaw/pkg/config"
)

type languageModelProvider interface {
	LanguageModel(ctx context.Context, modelID string) (core.LanguageModel, error)
}

type Client struct {
	provider        languageModelProvider
	requestTimeout  time.Duration
	modelID         string
	maxOutputTokens *int64
	temperature     *float64
	generate        func(context.Context, core.LanguageModel, core.AgentCall) (*core.AgentResult, error)

	mu            sync.RWMutex
	nextSessionID uint64
	sessions      map[string][]core.Message
}

func New(cfg *config.Config) (*Client, error) {
	if strings.TrimSpace(cfg.Agents.Defaults.Provider) != "openai" {
		return nil, fmt.Errorf("fantasy-agent currently supports only provider openai, got %q", cfg.Agents.Defaults.Provider)
	}

	apiKey := resolveAPIKey(cfg.Providers.OpenAI)
	if apiKey == "" {
		return nil, errors.New("providers.openai.api_key_env is required or OPENAI_API_KEY must be set")
	}

	modelID, err := normalizeOpenAIModel(cfg.Agents.Defaults.Model)
	if err != nil {
		return nil, err
	}

	providerOptions := []provideropenai.Option{provideropenai.WithAPIKey(apiKey)}
	if baseURL := strings.TrimSpace(cfg.Providers.OpenAI.BaseURL); baseURL != "" {
		providerOptions = append(providerOptions, provideropenai.WithBaseURL(baseURL))
	}
	if organization := strings.TrimSpace(cfg.Providers.OpenAI.Organization); organization != "" {
		providerOptions = append(providerOptions, provideropenai.WithOrganization(organization))
	}
	if project := strings.TrimSpace(cfg.Providers.OpenAI.Project); project != "" {
		providerOptions = append(providerOptions, provideropenai.WithProject(project))
	}

	fantasyProvider, err := provideropenai.New(providerOptions...)
	if err != nil {
		return nil, fmt.Errorf("initialize fantasy openai provider: %w", err)
	}

	requestTimeout := time.Duration(cfg.Providers.OpenAI.RequestTimeoutSeconds) * time.Second

	client := &Client{
		provider:       fantasyProvider,
		requestTimeout: requestTimeout,
		modelID:        modelID,
		sessions:       make(map[string][]core.Message),
		generate:       generateWithFantasyAgent,
	}

	if cfg.Agents.Defaults.MaxTokens > 0 {
		maxTokens := int64(cfg.Agents.Defaults.MaxTokens)
		client.maxOutputTokens = &maxTokens
	}
	if cfg.Agents.Defaults.Temperature > 0 {
		temp := cfg.Agents.Defaults.Temperature
		client.temperature = &temp
	}

	return client, nil
}

func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.provider.LanguageModel(ctx, c.modelID); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

func (c *Client) CreateSession(ctx context.Context, title string) (string, error) {
	_ = title

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextSessionID++
	sessionID := "fantasy-session-" + strconv.FormatUint(c.nextSessionID, 10)
	c.sessions[sessionID] = nil

	return sessionID, nil
}

func (c *Client) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string) (string, error) {
	_ = agent

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session id is required")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}

	modelID, err := normalizeOpenAIModel(model)
	if err != nil {
		return "", err
	}

	history, ok := c.sessionHistory(sessionID)
	if !ok {
		return "", errors.New("session is not started")
	}

	languageModel, err := c.provider.LanguageModel(ctx, modelID)
	if err != nil {
		return "", fmt.Errorf("resolve language model: %w", err)
	}

	call := core.AgentCall{
		Prompt:   prompt,
		Messages: history,
	}
	if c.maxOutputTokens != nil {
		call.MaxOutputTokens = c.maxOutputTokens
	}
	if c.temperature != nil {
		call.Temperature = c.temperature
	}

	generate := c.generate
	if generate == nil {
		generate = generateWithFantasyAgent
	}

	result, err := generate(ctx, languageModel, call)
	if err != nil {
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	response := extractText(result.Response.Content)
	if response == "" {
		return "", errors.New("prompt succeeded but returned no text")
	}

	c.appendSessionMessages(sessionID,
		core.NewUserMessage(prompt),
		core.Message{
			Role: core.MessageRoleAssistant,
			Content: []core.MessagePart{
				core.TextPart{Text: response},
			},
		},
	)

	return response, nil
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, c.requestTimeout)
}

func (c *Client) sessionHistory(sessionID string) ([]core.Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	history, ok := c.sessions[sessionID]
	if !ok {
		return nil, false
	}

	copyHistory := make([]core.Message, len(history))
	copy(copyHistory, history)
	return copyHistory, true
}

func (c *Client) appendSessionMessages(sessionID string, messages ...core.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	history, ok := c.sessions[sessionID]
	if !ok {
		return
	}

	history = append(history, messages...)
	c.sessions[sessionID] = history
}

func resolveAPIKey(cfg config.OpenAIProviderConfig) string {
	if apiKeyEnv := strings.TrimSpace(cfg.APIKeyEnv); apiKeyEnv != "" {
		if apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv)); apiKey != "" {
			return apiKey
		}
	}

	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

func normalizeOpenAIModel(model string) (string, error) {
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
		return "", fmt.Errorf("model provider %q is not supported by fantasy openai provider", providerID)
	}

	return modelID, nil
}

func extractText(content core.ResponseContent) string {
	lines := make([]string, 0)
	for _, part := range content {
		if part.GetType() != core.ContentTypeText {
			continue
		}

		textPart, ok := core.AsContentType[core.TextContent](part)
		if !ok {
			continue
		}

		line := strings.TrimSpace(textPart.Text)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func generateWithFantasyAgent(ctx context.Context, model core.LanguageModel, call core.AgentCall) (*core.AgentResult, error) {
	runtime := core.NewAgent(model)
	return runtime.Generate(ctx, call)
}
