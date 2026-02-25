package runtime

import (
	"strconv"
	"strings"

	"miniclaw/pkg/bus"
	providertypes "miniclaw/pkg/provider/types"
)

const (
	UsageInputTokensKey       = "usage_input_tokens"
	UsageOutputTokensKey      = "usage_output_tokens"
	UsageTotalTokensKey       = "usage_total_tokens"
	UsageReasoningTokensKey   = "usage_reasoning_tokens"
	UsageCacheCreateTokensKey = "usage_cache_creation_tokens"
	UsageCacheReadTokensKey   = "usage_cache_read_tokens"
)

// PromptResultMetadata serializes provider usage fields into outbound metadata.
//
// Keeping this logic in one place avoids subtle drift between CLI and gateway
// response formatting.
func PromptResultMetadata(result providertypes.PromptResult) map[string]string {
	if result.Metadata.Usage == nil {
		return nil
	}

	usage := result.Metadata.Usage
	return map[string]string{
		UsageInputTokensKey:       strconv.FormatInt(usage.InputTokens, 10),
		UsageOutputTokensKey:      strconv.FormatInt(usage.OutputTokens, 10),
		UsageTotalTokensKey:       strconv.FormatInt(usage.TotalTokens, 10),
		UsageReasoningTokensKey:   strconv.FormatInt(usage.ReasoningTokens, 10),
		UsageCacheCreateTokensKey: strconv.FormatInt(usage.CacheCreationTokens, 10),
		UsageCacheReadTokensKey:   strconv.FormatInt(usage.CacheReadTokens, 10),
	}
}

// PromptResultFromOutbound reconstructs provider usage from bus metadata.
func PromptResultFromOutbound(outbound bus.OutboundMessage) providertypes.PromptResult {
	result := providertypes.PromptResult{Text: outbound.Content}
	if outbound.Metadata == nil {
		return result
	}

	usage := &providertypes.TokenUsage{
		InputTokens:         parseInt64(outbound.Metadata[UsageInputTokensKey]),
		OutputTokens:        parseInt64(outbound.Metadata[UsageOutputTokensKey]),
		TotalTokens:         parseInt64(outbound.Metadata[UsageTotalTokensKey]),
		ReasoningTokens:     parseInt64(outbound.Metadata[UsageReasoningTokensKey]),
		CacheCreationTokens: parseInt64(outbound.Metadata[UsageCacheCreateTokensKey]),
		CacheReadTokens:     parseInt64(outbound.Metadata[UsageCacheReadTokensKey]),
	}

	if usage.IsZero() {
		return result
	}

	result.Metadata.Usage = usage
	return result
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}

	return parsed
}
