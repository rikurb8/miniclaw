package cmd

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestIsExitCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "exit", want: true},
		{input: " quit ", want: true},
		{input: ":q", want: true},
		{input: "EXIT", want: true},
		{input: "hello", want: false},
		{input: "quit now", want: false},
	}

	for _, tt := range tests {
		if got := isExitCommand(tt.input); got != tt.want {
			t.Fatalf("isExitCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAssistantLines(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOut []string
	}{
		{name: "single line", input: "hello", wantOut: []string{"hello"}},
		{name: "multi line", input: "one\ntwo", wantOut: []string{"one", "two"}},
		{name: "trim outer whitespace", input: "  one\ntwo  ", wantOut: []string{"one", "two"}},
		{name: "empty input", input: "   ", wantOut: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assistantLines(tt.input)
			if !reflect.DeepEqual(got, tt.wantOut) {
				t.Fatalf("assistantLines(%q) = %#v, want %#v", tt.input, got, tt.wantOut)
			}
		})
	}
}

func TestResolvePrompt(t *testing.T) {
	original := promptText
	t.Cleanup(func() {
		promptText = original
	})

	promptText = " from-flag "
	if got := resolvePrompt([]string{"from", "args"}); got != "from-flag" {
		t.Fatalf("resolvePrompt with flag = %q, want %q", got, "from-flag")
	}

	promptText = ""
	if got := resolvePrompt([]string{"hello", "world"}); got != "hello world" {
		t.Fatalf("resolvePrompt with args = %q, want %q", got, "hello world")
	}

	if got := resolvePrompt(nil); got != "" {
		t.Fatalf("resolvePrompt without input = %q, want empty", got)
	}
}

func TestPrintAssistantMessage(t *testing.T) {
	output := captureStdout(t, func() {
		printAssistantMessage("first\nsecond")
	})

	if output != "🦞 first\n🦞 second\n\n" {
		t.Fatalf("printAssistantMessage output = %q", output)
	}

	emptyOutput := captureStdout(t, func() {
		printAssistantMessage("   ")
	})
	if emptyOutput != "" {
		t.Fatalf("expected no output for empty message, got %q", emptyOutput)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stdout = w

	outCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		var builder strings.Builder
		_, copyErr := io.Copy(&builder, r)
		if copyErr != nil {
			errCh <- copyErr
			return
		}
		outCh <- builder.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = original

	select {
	case copyErr := <-errCh:
		_ = r.Close()
		t.Fatalf("read captured stdout: %v", copyErr)
	case output := <-outCh:
		_ = r.Close()
		return output
	}

	return ""
}
