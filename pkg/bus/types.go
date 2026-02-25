package bus

// InboundMessage is a normalized user/system message entering runtime processing.
type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage is a normalized message produced by runtime processing.
type OutboundMessage struct {
	Channel    string            `json:"channel"`
	ChatID     string            `json:"chat_id"`
	SessionKey string            `json:"session_key,omitempty"`
	Content    string            `json:"content"`
	Error      string            `json:"error,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// MessageHandler handles one inbound message for a specific channel.
type MessageHandler func(InboundMessage) error
