package fantasy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	core "charm.land/fantasy"
	provideropenai "charm.land/fantasy/providers/openai"

	"miniclaw/pkg/config"
	providertypes "miniclaw/pkg/provider/types"
	fantasytools "miniclaw/pkg/tools/fantasy"
	fstools "miniclaw/pkg/tools/fs"
	"miniclaw/pkg/workspace"
)

type languageModelProvider interface {
	LanguageModel(ctx context.Context, modelID string) (core.LanguageModel, error)
}

// Client is an in-memory session provider powered by charm.land/fantasy.
type Client struct {
	provider        languageModelProvider
	requestTimeout  time.Duration
	modelID         string
	maxOutputTokens *int64
	temperature     *float64
	generate        func(context.Context, core.LanguageModel, core.AgentCall, []core.AgentOption) (*core.AgentResult, error)
	tools           []core.AgentTool
	maxToolSteps    int

	mu            sync.RWMutex
	nextSessionID uint64
	sessions      map[string][]core.Message
}

// New constructs a fantasy-backed OpenAI provider client.
func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if strings.TrimSpace(cfg.Agents.Defaults.Provider) != "openai" {
		return nil, fmt.Errorf("fantasy-agent currently supports only provider openai, got %q", cfg.Agents.Defaults.Provider)
	}

	apiKey := resolveAPIKey()
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY must be set")
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

	guard, err := workspace.NewGuardWithPolicy(cfg.Agents.Defaults.Workspace, cfg.Agents.Defaults.RestrictToWorkspace)
	if err != nil {
		return nil, fmt.Errorf("initialize workspace guard: %w", err)
	}

	fsService := fstools.NewService(guard)
	tools := fantasytools.BuildFSTools(fsService, guard)
	maxToolSteps := cfg.Agents.Defaults.MaxToolIterations
	if maxToolSteps <= 0 {
		maxToolSteps = 20
	}

	client := &Client{
		provider:       fantasyProvider,
		requestTimeout: requestTimeout,
		modelID:        modelID,
		tools:          tools,
		maxToolSteps:   maxToolSteps,
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

// Health verifies that the configured model can be resolved.
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	if _, err := c.provider.LanguageModel(ctx, c.modelID); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// CreateSession allocates an in-memory session identifier.
func (c *Client) CreateSession(ctx context.Context, title string) (string, error) {
	// The fantasy provider keeps sessions in-memory only; title is currently informational.
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

// Prompt executes one prompt against the selected model and updates session history.
func (c *Client) Prompt(ctx context.Context, sessionID string, prompt string, model string, agent string, systemPrompt string) (providertypes.PromptResult, error) {
	_ = agent

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return providertypes.PromptResult{}, errors.New("session id is required")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return providertypes.PromptResult{}, errors.New("prompt is required")
	}

	modelID, err := normalizeOpenAIModel(model)
	if err != nil {
		return providertypes.PromptResult{}, err
	}

	history, ok := c.sessionHistory(sessionID)
	if !ok {
		return providertypes.PromptResult{}, errors.New("session is not started")
	}

	trimmedSystemPrompt := strings.TrimSpace(systemPrompt)
	if trimmedSystemPrompt != "" && len(history) == 0 {
		systemMessage := core.Message{
			Role: core.MessageRoleSystem,
			Content: []core.MessagePart{
				core.TextPart{Text: trimmedSystemPrompt},
			},
		}
		history = append(history, systemMessage)
		c.appendSessionMessages(sessionID, systemMessage)
	}

	languageModel, err := c.provider.LanguageModel(ctx, modelID)
	if err != nil {
		return providertypes.PromptResult{}, fmt.Errorf("resolve language model: %w", err)
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

	agentOptions := c.buildAgentOptions()
	result, err := generate(ctx, languageModel, call, agentOptions)
	if err != nil {
		return providertypes.PromptResult{}, fmt.Errorf("prompt failed: %w", err)
	}

	if c.shouldFinalizeAfterLimit(result) {
		finalized, finalizeErr := c.generateFinalSummaryStep(ctx, languageModel, history, prompt, result, agentOptions)
		if finalizeErr != nil {
			return providertypes.PromptResult{}, fmt.Errorf("finalize limited tool run: %w", finalizeErr)
		}
		result = finalized
	}

	response := extractText(result.Response.Content)
	if response == "" {
		return providertypes.PromptResult{}, errors.New("prompt succeeded but returned no text")
	}

	messagesToAppend := []core.Message{core.NewUserMessage(prompt)}
	if len(c.tools) > 0 {
		stepHistory := stepMessages(result.Steps)
		if len(stepHistory) > 0 {
			messagesToAppend = append(messagesToAppend, stepHistory...)
		} else {
			messagesToAppend = append(messagesToAppend, core.Message{
				Role: core.MessageRoleAssistant,
				Content: []core.MessagePart{
					core.TextPart{Text: response},
				},
			})
		}
	} else {
		messagesToAppend = append(messagesToAppend, core.Message{
			Role: core.MessageRoleAssistant,
			Content: []core.MessagePart{
				core.TextPart{Text: response},
			},
		})
	}
	c.appendSessionMessages(sessionID, messagesToAppend...)

	usage := providertypes.TokenUsage{
		InputTokens:         result.TotalUsage.InputTokens,
		OutputTokens:        result.TotalUsage.OutputTokens,
		TotalTokens:         result.TotalUsage.TotalTokens,
		ReasoningTokens:     result.TotalUsage.ReasoningTokens,
		CacheCreationTokens: result.TotalUsage.CacheCreationTokens,
		CacheReadTokens:     result.TotalUsage.CacheReadTokens,
	}

	metadata := providertypes.PromptMetadata{
		Provider: "openai",
		Model:    modelID,
		Agent:    strings.TrimSpace(agent),
	}
	if !usage.IsZero() {
		metadata.Usage = &usage
	}

	return providertypes.PromptResult{
		Text:     response,
		Metadata: metadata,
	}, nil
}

func (c *Client) buildAgentOptions() []core.AgentOption {
	if len(c.tools) == 0 {
		return nil
	}

	return []core.AgentOption{
		core.WithTools(c.tools...),
		core.WithStopConditions(core.StepCountIs(c.maxToolSteps)),
	}
}

func (c *Client) shouldFinalizeAfterLimit(result *core.AgentResult) bool {
	if len(c.tools) == 0 || result == nil || c.maxToolSteps <= 0 {
		return false
	}
	if len(result.Steps) < c.maxToolSteps {
		return false
	}

	lastStep := result.Steps[len(result.Steps)-1]
	return lastStep.FinishReason == core.FinishReasonToolCalls
}

func (c *Client) generateFinalSummaryStep(ctx context.Context, model core.LanguageModel, history []core.Message, userPrompt string, prior *core.AgentResult, agentOptions []core.AgentOption) (*core.AgentResult, error) {
	summaryMessages := make([]core.Message, 0, len(history)+len(prior.Steps)*2+1)
	summaryMessages = append(summaryMessages, history...)
	summaryMessages = append(summaryMessages, core.NewUserMessage(userPrompt))
	summaryMessages = append(summaryMessages, stepMessages(prior.Steps)...)

	prepareNoTools := func(ctx context.Context, _ core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		return ctx, core.PrepareStepResult{DisableAllTools: true}, nil
	}

	finalOptions := append([]core.AgentOption{}, agentOptions...)
	finalOptions = append(finalOptions,
		core.WithStopConditions(core.StepCountIs(1)),
		core.WithPrepareStep(prepareNoTools),
	)

	finalCall := core.AgentCall{
		Prompt:   "Provide a concise final answer to the user based on the completed tool results.",
		Messages: summaryMessages,
	}
	if c.maxOutputTokens != nil {
		finalCall.MaxOutputTokens = c.maxOutputTokens
	}
	if c.temperature != nil {
		finalCall.Temperature = c.temperature
	}

	generate := c.generate
	if generate == nil {
		generate = generateWithFantasyAgent
	}

	finalResult, err := generate(ctx, model, finalCall, finalOptions)
	if err != nil {
		return nil, err
	}

	mergedSteps := make([]core.StepResult, 0, len(prior.Steps)+len(finalResult.Steps))
	mergedSteps = append(mergedSteps, prior.Steps...)
	mergedSteps = append(mergedSteps, finalResult.Steps...)

	totalUsage := addUsage(prior.TotalUsage, finalResult.TotalUsage)

	merged := &core.AgentResult{
		Steps:      mergedSteps,
		Response:   finalResult.Response,
		TotalUsage: totalUsage,
	}

	slog.Default().With("component", "provider.fantasy").Debug("Tool iteration limit reached; generated final summary step",
		"max_tool_iterations", c.maxToolSteps,
	)

	return merged, nil
}

func addUsage(a core.Usage, b core.Usage) core.Usage {
	return core.Usage{
		InputTokens:         a.InputTokens + b.InputTokens,
		OutputTokens:        a.OutputTokens + b.OutputTokens,
		TotalTokens:         a.TotalTokens + b.TotalTokens,
		ReasoningTokens:     a.ReasoningTokens + b.ReasoningTokens,
		CacheCreationTokens: a.CacheCreationTokens + b.CacheCreationTokens,
		CacheReadTokens:     a.CacheReadTokens + b.CacheReadTokens,
	}
}

func stepMessages(steps []core.StepResult) []core.Message {
	messages := make([]core.Message, 0, len(steps)*2)
	for _, step := range steps {
		if len(step.Messages) == 0 {
			continue
		}
		messages = append(messages, step.Messages...)
	}

	return messages
}

// withTimeout wraps context with provider-level request timeout when configured.
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, c.requestTimeout)
}

// sessionHistory returns a defensive copy of session messages.
func (c *Client) sessionHistory(sessionID string) ([]core.Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	history, ok := c.sessions[sessionID]
	if !ok {
		return nil, false
	}

	// Copy so callers cannot mutate shared session history.
	copyHistory := make([]core.Message, len(history))
	copy(copyHistory, history)
	return copyHistory, true
}

// appendSessionMessages appends messages to one tracked in-memory session.
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

// resolveAPIKey reads OPENAI_API_KEY from environment.
func resolveAPIKey() string {
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

// normalizeOpenAIModel accepts bare model IDs or openai/<model> references.
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

// extractText collects non-empty text response parts into a single string.
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

// generateWithFantasyAgent delegates prompt generation to fantasy runtime.
func generateWithFantasyAgent(ctx context.Context, model core.LanguageModel, call core.AgentCall, options []core.AgentOption) (*core.AgentResult, error) {
	runtime := core.NewAgent(model, options...)
	return runtime.Generate(ctx, call)
}
