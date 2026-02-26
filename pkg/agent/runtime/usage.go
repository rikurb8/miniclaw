package runtime

import (
	"encoding/json"
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
	ToolEventsJSONKey         = "tool_events_json"
)

// PromptResultMetadata serializes provider usage fields into outbound metadata.
//
// Keeping this logic in one place avoids subtle drift between CLI and gateway
// response formatting.
func PromptResultMetadata(result providertypes.PromptResult) map[string]string {
	if result.Metadata.Usage == nil && len(result.Metadata.ToolEvents) == 0 {
		return nil
	}

	metadata := map[string]string{}
	if result.Metadata.Usage != nil {
		usage := result.Metadata.Usage
		metadata[UsageInputTokensKey] = strconv.FormatInt(usage.InputTokens, 10)
		metadata[UsageOutputTokensKey] = strconv.FormatInt(usage.OutputTokens, 10)
		metadata[UsageTotalTokensKey] = strconv.FormatInt(usage.TotalTokens, 10)
		metadata[UsageReasoningTokensKey] = strconv.FormatInt(usage.ReasoningTokens, 10)
		metadata[UsageCacheCreateTokensKey] = strconv.FormatInt(usage.CacheCreationTokens, 10)
		metadata[UsageCacheReadTokensKey] = strconv.FormatInt(usage.CacheReadTokens, 10)
	}

	if len(result.Metadata.ToolEvents) > 0 {
		payload, err := json.Marshal(result.Metadata.ToolEvents)
		if err == nil {
			metadata[ToolEventsJSONKey] = string(payload)
		}
	}

	if len(metadata) == 0 {
		return nil
	}

	return metadata
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
		usage = nil
	}

	result.Metadata.Usage = usage
	if raw, ok := outbound.Metadata[ToolEventsJSONKey]; ok {
		result.Metadata.ToolEvents = parseToolEvents(raw)
	}

	return result
}

func parseToolEvents(raw string) []providertypes.ToolEvent {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var events []providertypes.ToolEvent
	if err := json.Unmarshal([]byte(trimmed), &events); err != nil {
		return nil
	}

	if len(events) == 0 {
		return nil
	}

	return events
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}

	return parsed
}
