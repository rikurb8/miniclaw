package types

// PromptResult is the normalized provider response payload.
type PromptResult struct {
	Text     string
	Metadata PromptMetadata
}

// PromptMetadata carries provider/model identity and optional usage accounting.
type PromptMetadata struct {
	Provider string
	Model    string
	Agent    string
	Usage    *TokenUsage
}

// TokenUsage captures token accounting across providers.
type TokenUsage struct {
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	ReasoningTokens     int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// IsZero reports whether all token counters are unset/zero.
func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 &&
		u.OutputTokens == 0 &&
		u.TotalTokens == 0 &&
		u.ReasoningTokens == 0 &&
		u.CacheCreationTokens == 0 &&
		u.CacheReadTokens == 0
}
