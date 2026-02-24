package opencode

import (
	"strings"
	"testing"

	"miniclaw/pkg/config"

	sdk "github.com/sst/opencode-sdk-go"
)

func TestNewRequiresBaseURL(t *testing.T) {
	cfg := &config.Config{}

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error when base_url is missing")
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOK     bool
		wantProvID string
		wantModel  string
	}{
		{name: "valid", input: "openai/gpt-5.2", wantOK: true, wantProvID: "openai", wantModel: "gpt-5.2"},
		{name: "missing slash", input: "gpt-5.2", wantOK: false},
		{name: "empty provider", input: "/gpt-5.2", wantOK: false},
		{name: "empty model", input: "openai/", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provID, modelID, ok := parseModelRef(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if provID != tt.wantProvID {
				t.Fatalf("providerID = %q, want %q", provID, tt.wantProvID)
			}
			if modelID != tt.wantModel {
				t.Fatalf("modelID = %q, want %q", modelID, tt.wantModel)
			}
		})
	}
}

func TestExtractText(t *testing.T) {
	parts := []sdk.Part{
		{Type: sdk.PartTypeReasoning, Text: "should be ignored"},
		{Type: sdk.PartTypeText, Text: "  first line  "},
		{Type: sdk.PartTypeText, Text: ""},
		{Type: sdk.PartTypeText, Text: "second line"},
	}

	got := extractText(parts)
	if got != "first line\nsecond line" {
		t.Fatalf("extractText() = %q", got)
	}
}

func TestBuildBasicAuthHeader(t *testing.T) {
	t.Setenv("TEST_OPENCODE_PASSWORD", "secret")

	header, ok := buildBasicAuthHeader(config.OpenCodeProviderConfig{
		Username:    "opencode",
		PasswordEnv: "TEST_OPENCODE_PASSWORD",
	})
	if !ok {
		t.Fatal("expected basic auth header")
	}
	if !strings.HasPrefix(header, "Basic ") {
		t.Fatalf("unexpected header prefix: %q", header)
	}
}

func TestBuildBasicAuthHeaderMissingEnvValue(t *testing.T) {
	t.Setenv("TEST_OPENCODE_PASSWORD_EMPTY", "")

	_, ok := buildBasicAuthHeader(config.OpenCodeProviderConfig{
		PasswordEnv: "TEST_OPENCODE_PASSWORD_EMPTY",
	})
	if ok {
		t.Fatal("expected no basic auth header")
	}
}
