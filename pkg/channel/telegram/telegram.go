package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"miniclaw/pkg/bus"
	"miniclaw/pkg/channel"
	"miniclaw/pkg/config"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const channelName = "telegram"
const messagePreviewLimit = 240
const typingRefreshInterval = 4 * time.Second

// Adapter bridges Telegram updates into MiniClaw inbound/outbound messages.
type Adapter struct {
	cfg       config.TelegramConfig
	allowFrom map[string]struct{}
	log       *slog.Logger
}

// NewAdapter validates Telegram configuration and constructs an adapter instance.
func NewAdapter(cfg config.TelegramConfig, log *slog.Logger) (*Adapter, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("channels.telegram.token is required")
	}

	if log == nil {
		log = slog.Default()
	}

	return &Adapter{
		cfg:       cfg,
		allowFrom: allowFromSet(cfg.AllowFrom),
		log:       log.With("component", "channel.telegram"),
	}, nil
}

// Name returns the channel identifier used in bus metadata and logs.
func (a *Adapter) Name() string {
	return channelName
}

// Run starts Telegram long polling and forwards messages through the shared channel handler.
func (a *Adapter) Run(ctx context.Context, handler channel.Handler) error {
	if handler == nil {
		return errors.New("handler is required")
	}

	bot, err := telego.NewBot(strings.TrimSpace(a.cfg.Token))
	if err != nil {
		return fmt.Errorf("initialize telegram bot: %w", err)
	}

	updates, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("start long polling: %w", err)
	}

	a.log.Info("Telegram channel started")

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				if err := ctx.Err(); err != nil {
					return nil
				}
				return errors.New("telegram updates channel closed")
			}

			message := update.Message
			if message == nil {
				continue
			}

			content := strings.TrimSpace(message.Text)
			if content == "" {
				// Ignore non-text updates for now; runtime currently expects text content.
				continue
			}
			if message.From == nil {
				a.log.Debug("Ignoring message without sender")
				continue
			}

			senderID := strconv.FormatInt(message.From.ID, 10)
			if !a.senderAllowed(senderID) {
				a.log.Debug("Ignoring message from unauthorized sender", "sender_id", senderID)
				continue
			}

			chatID := strconv.FormatInt(message.Chat.ID, 10)
			inbound := bus.InboundMessage{
				Channel:    channelName,
				SenderID:   senderID,
				ChatID:     chatID,
				SessionKey: sessionKey(chatID),
				Content:    content,
				Metadata: map[string]string{
					"update_id": strconv.Itoa(update.UpdateID),
				},
			}
			a.log.Info("Received message", "chat_id", chatID, "sender_id", senderID, "session_key", inbound.SessionKey, "content", previewText(content))

			stopTyping := a.startTypingIndicator(ctx, bot, message.Chat.ID)

			outbound, err := handler(ctx, inbound)
			stopTyping()
			if err != nil {
				a.log.Error("Failed to process inbound message", "error", err)
				outbound = bus.OutboundMessage{Error: err.Error()}
			}

			responseText := strings.TrimSpace(outbound.Content)
			if responseText == "" {
				responseText = strings.TrimSpace(outbound.Error)
			}
			if responseText == "" {
				continue
			}
			a.log.Info("Sending message", "chat_id", chatID, "session_key", inbound.SessionKey, "content", previewText(responseText))

			if _, err := bot.SendMessage(ctx, tu.Message(tu.ID(message.Chat.ID), responseText)); err != nil {
				a.log.Error("Failed to send telegram message", "error", err)
			}
		}
	}
}

// senderAllowed checks whether a sender is permitted by allow_from config.
//
// When no allow list is configured, all senders are accepted.
func (a *Adapter) senderAllowed(senderID string) bool {
	if len(a.allowFrom) == 0 {
		return true
	}

	_, ok := a.allowFrom[strings.TrimSpace(senderID)]
	return ok
}

// sessionKey maps one Telegram chat to one runtime session namespace.
func sessionKey(chatID string) string {
	return "telegram:" + strings.TrimSpace(chatID)
}

// allowFromSet normalizes allow_from values into a lookup set.
func allowFromSet(allowFrom []string) map[string]struct{} {
	if len(allowFrom) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(allowFrom))
	for _, value := range allowFrom {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	if len(allowed) == 0 {
		return nil
	}

	return allowed
}

// previewText returns a bounded log-safe preview of message text.
func previewText(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= messagePreviewLimit {
		return trimmed
	}

	return trimmed[:messagePreviewLimit] + "..."
}

// startTypingIndicator sends an initial typing action and refreshes it periodically
// until the returned cancel function is called.
func (a *Adapter) startTypingIndicator(ctx context.Context, bot *telego.Bot, chatID int64) context.CancelFunc {
	typingCtx, cancel := context.WithCancel(ctx)

	sendTyping := func() {
		if err := bot.SendChatAction(typingCtx, tu.ChatAction(tu.ID(chatID), telego.ChatActionTyping)); err != nil && typingCtx.Err() == nil {
			a.log.Debug("Failed to send typing indicator", "chat_id", chatID, "error", err)
		}
	}

	sendTyping()

	go func() {
		ticker := time.NewTicker(typingRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				sendTyping()
			}
		}
	}()

	return cancel
}
