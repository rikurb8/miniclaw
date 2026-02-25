package profile

import "testing"

func TestResolveSystemProfile(t *testing.T) {
	t.Run("opencode returns empty profile", func(t *testing.T) {
		content, err := ResolveSystemProfile("opencode")
		if err != nil {
			t.Fatalf("ResolveSystemProfile error: %v", err)
		}
		if content != "" {
			t.Fatalf("content = %q, want empty", content)
		}
	})

	t.Run("non-opencode returns default profile", func(t *testing.T) {
		content, err := ResolveSystemProfile("openai")
		if err != nil {
			t.Fatalf("ResolveSystemProfile error: %v", err)
		}
		if content == "" {
			t.Fatal("expected non-empty profile content")
		}
	})
}

func TestTemplatePath(t *testing.T) {
	if got := templatePath("default"); got != "templates/default.md" {
		t.Fatalf("templatePath(default) = %q, want %q", got, "templates/default.md")
	}
}
