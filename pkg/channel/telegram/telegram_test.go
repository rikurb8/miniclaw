package telegram

import (
	"strings"
	"testing"
)

func TestAllowFromSet(t *testing.T) {
	allowed := allowFromSet([]string{" 123 ", "", "456", "123"})
	if len(allowed) != 2 {
		t.Fatalf("allowFromSet len = %d, want 2", len(allowed))
	}
	if _, ok := allowed["123"]; !ok {
		t.Fatal("allowFromSet missing 123")
	}
	if _, ok := allowed["456"]; !ok {
		t.Fatal("allowFromSet missing 456")
	}
}

func TestSenderAllowed(t *testing.T) {
	adapter := &Adapter{allowFrom: map[string]struct{}{"1": {}}}
	if !adapter.senderAllowed("1") {
		t.Fatal("expected sender 1 to be allowed")
	}
	if adapter.senderAllowed("2") {
		t.Fatal("expected sender 2 to be denied")
	}

	adapter.allowFrom = nil
	if !adapter.senderAllowed("any") {
		t.Fatal("expected sender to be allowed when allowlist empty")
	}
}

func TestSessionKey(t *testing.T) {
	if got := sessionKey(" 42 "); got != "telegram:42" {
		t.Fatalf("sessionKey = %q, want %q", got, "telegram:42")
	}
}

func TestPreviewText(t *testing.T) {
	short := " hello "
	if got := previewText(short); got != "hello" {
		t.Fatalf("previewText short = %q, want %q", got, "hello")
	}

	long := strings.Repeat("a", messagePreviewLimit+20)
	got := previewText(long)
	if len(got) != messagePreviewLimit+3 {
		t.Fatalf("previewText long len = %d, want %d", len(got), messagePreviewLimit+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("previewText long = %q, want ellipsis suffix", got)
	}
}
