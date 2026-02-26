package fantasy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	core "charm.land/fantasy"

	"miniclaw/pkg/config"
)

type fakeLanguageModelProvider struct {
	model     core.LanguageModel
	err       error
	lastID    string
	callCount int
}

func (f *fakeLanguageModelProvider) LanguageModel(ctx context.Context, modelID string) (core.LanguageModel, error) {
	f.callCount++
	f.lastID = modelID
	if f.err != nil {
		return nil, f.err
	}

	return f.model, nil
}

type fakeLanguageModel struct{}

func (f *fakeLanguageModel) Generate(context.Context, core.Call) (*core.Response, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLanguageModel) Stream(context.Context, core.Call) (core.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLanguageModel) GenerateObject(context.Context, core.ObjectCall) (*core.ObjectResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLanguageModel) StreamObject(context.Context, core.ObjectCall) (core.ObjectStreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeLanguageModel) Provider() string { return "openai" }
func (f *fakeLanguageModel) Model() string    { return "gpt-5.2" }

func TestNewRejectsNonOpenAIProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg := &config.Config{}
	cfg.Agents.Defaults.Provider = "opencode"
	cfg.Agents.Defaults.Model = "openai/gpt-5.2"

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	cfg := &config.Config{}
	cfg.Agents.Defaults.Provider = "openai"
	cfg.Agents.Defaults.Model = "openai/gpt-5.2"

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestNewInitializesToolsAndDefaultIterationLimit(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg := &config.Config{}
	cfg.Agents.Defaults.Provider = "openai"
	cfg.Agents.Defaults.Model = "openai/gpt-5.2"
	cfg.Agents.Defaults.Workspace = filepath.Join(t.TempDir(), "workspace")
	cfg.Agents.Defaults.MaxToolIterations = 0

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if len(client.tools) != 5 {
		t.Fatalf("tools length = %d, want 5", len(client.tools))
	}
	if client.maxToolSteps != 20 {
		t.Fatalf("maxToolSteps = %d, want 20", client.maxToolSteps)
	}
}

func TestNormalizeOpenAIModel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain model", input: "gpt-5.2", want: "gpt-5.2"},
		{name: "openai prefixed", input: "openai/gpt-5.2", want: "gpt-5.2"},
		{name: "non openai prefixed", input: "anthropic/claude", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOpenAIModel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeOpenAIModel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeOpenAIModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCreateSessionAndHealth(t *testing.T) {
	provider := &fakeLanguageModelProvider{model: &fakeLanguageModel{}}
	client := &Client{
		provider: provider,
		modelID:  "gpt-5.2",
		sessions: map[string][]core.Message{},
	}

	sessionID, err := client.CreateSession(context.Background(), "title")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session id")
	}

	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health error: %v", err)
	}
	if provider.callCount != 1 {
		t.Fatalf("health call count = %d, want 1", provider.callCount)
	}
	if provider.lastID != "gpt-5.2" {
		t.Fatalf("model id = %q, want %q", provider.lastID, "gpt-5.2")
	}
}

func TestPromptValidatesSessionAndInput(t *testing.T) {
	client := &Client{
		provider: &fakeLanguageModelProvider{model: &fakeLanguageModel{}},
		modelID:  "gpt-5.2",
		sessions: map[string][]core.Message{},
	}

	if _, err := client.Prompt(context.Background(), "", "hello", "gpt-5.2", "", ""); err == nil {
		t.Fatal("expected error for empty session")
	}
	if _, err := client.Prompt(context.Background(), "missing", "hello", "gpt-5.2", "", ""); err == nil {
		t.Fatal("expected error for missing session")
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if _, err := client.Prompt(context.Background(), sessionID, "", "gpt-5.2", "", ""); err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestPromptMaintainsSessionHistory(t *testing.T) {
	provider := &fakeLanguageModelProvider{model: &fakeLanguageModel{}}
	generationCalls := 0
	client := &Client{
		provider: provider,
		modelID:  "gpt-5.2",
		sessions: map[string][]core.Message{},
		generate: func(ctx context.Context, model core.LanguageModel, call core.AgentCall, _ []core.AgentOption) (*core.AgentResult, error) {
			generationCalls++
			return &core.AgentResult{
				Response: core.Response{
					Content: core.ResponseContent{
						core.TextContent{Text: fmt.Sprintf("reply-%d", generationCalls)},
					},
				},
			}, nil
		},
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	first, err := client.Prompt(context.Background(), sessionID, "hello", "gpt-5.2", "", "")
	if err != nil {
		t.Fatalf("first Prompt error: %v", err)
	}
	if first.Text != "reply-1" {
		t.Fatalf("first response = %q, want %q", first.Text, "reply-1")
	}

	second, err := client.Prompt(context.Background(), sessionID, "how are you", "gpt-5.2", "", "")
	if err != nil {
		t.Fatalf("second Prompt error: %v", err)
	}
	if second.Text != "reply-2" {
		t.Fatalf("second response = %q, want %q", second.Text, "reply-2")
	}

	history, ok := client.sessionHistory(sessionID)
	if !ok {
		t.Fatal("expected session history")
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4", len(history))
	}
	if history[0].Role != core.MessageRoleUser {
		t.Fatalf("first history role = %q, want %q", history[0].Role, core.MessageRoleUser)
	}
	if history[1].Role != core.MessageRoleAssistant {
		t.Fatalf("second history role = %q, want %q", history[1].Role, core.MessageRoleAssistant)
	}
}

func TestExtractText(t *testing.T) {
	content := core.ResponseContent{
		core.ReasoningContent{Text: "ignore me"},
		core.TextContent{Text: "  first  "},
		core.TextContent{Text: ""},
		core.TextContent{Text: "second"},
	}

	got := extractText(content)
	if got != "first\nsecond" {
		t.Fatalf("extractText() = %q", got)
	}
}

func TestPromptInjectsSystemMessageOnFirstTurn(t *testing.T) {
	provider := &fakeLanguageModelProvider{model: &fakeLanguageModel{}}
	var firstCallMessages []core.Message
	client := &Client{
		provider: provider,
		modelID:  "gpt-5.2",
		sessions: map[string][]core.Message{},
		generate: func(ctx context.Context, model core.LanguageModel, call core.AgentCall, _ []core.AgentOption) (*core.AgentResult, error) {
			if firstCallMessages == nil {
				firstCallMessages = call.Messages
			}
			return &core.AgentResult{
				Response: core.Response{
					Content: core.ResponseContent{core.TextContent{Text: "reply"}},
				},
			}, nil
		},
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	_, err = client.Prompt(context.Background(), sessionID, "hello", "gpt-5.2", "", "system profile")
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	if len(firstCallMessages) != 1 {
		t.Fatalf("messages length = %d, want 1", len(firstCallMessages))
	}
	if firstCallMessages[0].Role != core.MessageRoleSystem {
		t.Fatalf("first message role = %q, want %q", firstCallMessages[0].Role, core.MessageRoleSystem)
	}
}

func TestPromptPersistsStepMessagesWhenToolsEnabled(t *testing.T) {
	provider := &fakeLanguageModelProvider{model: &fakeLanguageModel{}}
	tool := core.NewAgentTool("noop", "noop tool", func(ctx context.Context, input struct{}, call core.ToolCall) (core.ToolResponse, error) {
		return core.NewTextResponse("ok"), nil
	})

	client := &Client{
		provider: provider,
		modelID:  "gpt-5.2",
		tools:    []core.AgentTool{tool},
		sessions: map[string][]core.Message{},
		generate: func(ctx context.Context, model core.LanguageModel, call core.AgentCall, _ []core.AgentOption) (*core.AgentResult, error) {
			return &core.AgentResult{
				Steps: []core.StepResult{
					{
						Messages: []core.Message{
							{Role: core.MessageRoleAssistant, Content: []core.MessagePart{core.TextPart{Text: "tool planning"}}},
							{Role: core.MessageRoleTool, Content: []core.MessagePart{core.ToolResultPart{ToolCallID: "1", Output: core.ToolResultOutputContentText{Text: "ok"}}}},
						},
						Response: core.Response{Content: core.ResponseContent{core.TextContent{Text: "final answer"}}},
					},
				},
				Response: core.Response{Content: core.ResponseContent{core.TextContent{Text: "final answer"}}},
			}, nil
		},
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	_, err = client.Prompt(context.Background(), sessionID, "hello", "gpt-5.2", "", "")
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	history, ok := client.sessionHistory(sessionID)
	if !ok {
		t.Fatal("expected session history")
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].Role != core.MessageRoleUser {
		t.Fatalf("history[0].Role = %q, want user", history[0].Role)
	}
	if history[1].Role != core.MessageRoleAssistant {
		t.Fatalf("history[1].Role = %q, want assistant", history[1].Role)
	}
	if history[2].Role != core.MessageRoleTool {
		t.Fatalf("history[2].Role = %q, want tool", history[2].Role)
	}
}

func TestPromptGeneratesFinalSummaryWhenToolLimitReached(t *testing.T) {
	provider := &fakeLanguageModelProvider{model: &fakeLanguageModel{}}
	tool := core.NewAgentTool("noop", "noop tool", func(ctx context.Context, input struct{}, call core.ToolCall) (core.ToolResponse, error) {
		return core.NewTextResponse("ok"), nil
	})

	calls := 0
	var secondCallPrompt string
	client := &Client{
		provider:     provider,
		modelID:      "gpt-5.2",
		tools:        []core.AgentTool{tool},
		maxToolSteps: 1,
		sessions:     map[string][]core.Message{},
		generate: func(ctx context.Context, model core.LanguageModel, call core.AgentCall, options []core.AgentOption) (*core.AgentResult, error) {
			calls++
			_ = model
			_ = options
			if calls == 1 {
				return &core.AgentResult{
					Steps: []core.StepResult{{
						Response: core.Response{
							FinishReason: core.FinishReasonToolCalls,
							Content:      core.ResponseContent{core.ToolCallContent{ToolCallID: "1", ToolName: "noop", Input: `{}`}},
						},
						Messages: []core.Message{{Role: core.MessageRoleAssistant, Content: []core.MessagePart{core.ToolCallPart{ToolCallID: "1", ToolName: "noop", Input: `{}`}}}},
					}},
					Response: core.Response{
						FinishReason: core.FinishReasonToolCalls,
						Content:      core.ResponseContent{core.ToolCallContent{ToolCallID: "1", ToolName: "noop", Input: `{}`}},
					},
				}, nil
			}

			secondCallPrompt = call.Prompt
			if len(call.Messages) == 0 {
				t.Fatal("expected summary call to include prior messages")
			}
			_ = ctx
			return &core.AgentResult{
				Steps: []core.StepResult{{
					Response: core.Response{Content: core.ResponseContent{core.TextContent{Text: "final summary"}}},
					Messages: []core.Message{{Role: core.MessageRoleAssistant, Content: []core.MessagePart{core.TextPart{Text: "final summary"}}}},
				}},
				Response: core.Response{Content: core.ResponseContent{core.TextContent{Text: "final summary"}}},
			}, nil
		},
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	result, err := client.Prompt(context.Background(), sessionID, "run tools", "gpt-5.2", "", "")
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if result.Text != "final summary" {
		t.Fatalf("result text = %q, want final summary", result.Text)
	}
	if calls != 2 {
		t.Fatalf("generate calls = %d, want 2", calls)
	}
	if secondCallPrompt == "" {
		t.Fatal("expected final summary prompt to be set")
	}
}

func TestBuildAgentOptionsIncludesToolsAndStepLimit(t *testing.T) {
	tool := core.NewAgentTool("noop", "noop tool", func(ctx context.Context, input struct{}, call core.ToolCall) (core.ToolResponse, error) {
		return core.NewTextResponse("ok"), nil
	})

	client := &Client{tools: []core.AgentTool{tool}, maxToolSteps: 3}
	options := client.buildAgentOptions()
	if len(options) != 2 {
		t.Fatalf("options length = %d, want 2", len(options))
	}

	model := &fakeLanguageModel{}
	agent := core.NewAgent(model, options...)
	_, err := agent.Generate(context.Background(), core.AgentCall{Prompt: "hello"})
	if err == nil {
		t.Fatal("expected generation error from fake model")
	}
}

func TestToolCallSerializationForHistoryMessages(t *testing.T) {
	content := core.ToolCallContent{ToolCallID: "1", ToolName: "read_file", Input: `{"path":"a.txt"}`}

	payload, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected serialized content")
	}
}
