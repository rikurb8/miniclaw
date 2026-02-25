package types

type PromptResult struct {
	Text     string
	Metadata PromptMetadata
}

type PromptMetadata struct {
	Provider string
	Model    string
	Agent    string
	Usage    *TokenUsage
}

type TokenUsage struct {
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	ReasoningTokens     int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 &&
		u.OutputTokens == 0 &&
		u.TotalTokens == 0 &&
		u.ReasoningTokens == 0 &&
		u.CacheCreationTokens == 0 &&
		u.CacheReadTokens == 0
}
