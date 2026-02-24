package fantasy

import (
	"context"
	"errors"
	"fmt"
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

	if _, err := client.Prompt(context.Background(), "", "hello", "gpt-5.2", ""); err == nil {
		t.Fatal("expected error for empty session")
	}
	if _, err := client.Prompt(context.Background(), "missing", "hello", "gpt-5.2", ""); err == nil {
		t.Fatal("expected error for missing session")
	}

	sessionID, err := client.CreateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if _, err := client.Prompt(context.Background(), sessionID, "", "gpt-5.2", ""); err == nil {
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
		generate: func(ctx context.Context, model core.LanguageModel, call core.AgentCall) (*core.AgentResult, error) {
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

	first, err := client.Prompt(context.Background(), sessionID, "hello", "gpt-5.2", "")
	if err != nil {
		t.Fatalf("first Prompt error: %v", err)
	}
	if first != "reply-1" {
		t.Fatalf("first response = %q, want %q", first, "reply-1")
	}

	second, err := client.Prompt(context.Background(), sessionID, "how are you", "gpt-5.2", "")
	if err != nil {
		t.Fatalf("second Prompt error: %v", err)
	}
	if second != "reply-2" {
		t.Fatalf("second response = %q, want %q", second, "reply-2")
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
