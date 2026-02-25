package gateway

import (
	"testing"
	"time"

	providertypes "miniclaw/pkg/provider/types"
)

func TestIsReady(t *testing.T) {
	t.Parallel()

	svc := &Service{channelStates: map[string]channelState{"telegram": {Running: true}}}
	if svc.isReady() {
		t.Fatal("expected not ready without provider health")
	}

	svc.providerLastOKAt = time.Now().UTC()
	if !svc.isReady() {
		t.Fatal("expected ready with running channel and healthy provider")
	}

	svc.providerLastErr = "boom"
	if svc.isReady() {
		t.Fatal("expected not ready when provider has error")
	}
}

func TestPromptResultMetadata(t *testing.T) {
	t.Parallel()

	metadata := promptResultMetadata(providertypes.PromptResult{
		Metadata: providertypes.PromptMetadata{
			Usage: &providertypes.TokenUsage{
				InputTokens:         10,
				OutputTokens:        11,
				TotalTokens:         21,
				ReasoningTokens:     5,
				CacheCreationTokens: 1,
				CacheReadTokens:     2,
			},
		},
	})

	if got := metadata[metaUsageInKey]; got != "10" {
		t.Fatalf("input tokens = %q, want 10", got)
	}
	if got := metadata[metaUsageTotalKey]; got != "21" {
		t.Fatalf("total tokens = %q, want 21", got)
	}
}
