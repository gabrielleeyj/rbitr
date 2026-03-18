package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	TelegramChannel    = "telegram"
	telegramAPIBaseURL = "https://api.telegram.org"
)

type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   http.DefaultClient,
	}
}

func (n *TelegramNotifier) Name() string {
	return TelegramChannel
}

func (n *TelegramNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	if n.botToken == "" {
		return errors.New("telegram bot token not configured")
	}
	if n.chatID == "" {
		return errors.New("telegram chat_id not configured")
	}

	text := buildTelegramText(msg)
	payload := telegramSendMessageRequest{
		ChatID:    n.chatID,
		Text:      text,
		ParseMode: "MarkdownV2",
	}

	body, err := json.Marshal(&payload)
	if err != nil {
		return fmt.Errorf("telegram marshal failed: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBaseURL, n.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := n.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API error: status %d", resp.StatusCode)
	}
	return nil
}

type telegramSendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func buildTelegramText(msg NotificationMessage) string {
	var b strings.Builder

	if msg.Title != "" {
		b.WriteString("*")
		b.WriteString(escapeTelegramMarkdown(msg.Title))
		b.WriteString("*\n")
	}
	if msg.Body != "" {
		b.WriteString(escapeTelegramMarkdown(msg.Body))
		b.WriteString("\n")
	}
	if len(msg.Fields) > 0 {
		b.WriteString("\n")
		keys := sortedFieldKeys(msg.Fields)
		for _, key := range keys {
			b.WriteString("*")
			b.WriteString(escapeTelegramMarkdown(key))
			b.WriteString("*: ")
			b.WriteString(escapeTelegramMarkdown(msg.Fields[key]))
			b.WriteString("\n")
		}
	}
	if len(msg.Links) > 0 {
		b.WriteString("\n")
		keys := sortedFieldKeys(msg.Links)
		for _, key := range keys {
			b.WriteString("[")
			b.WriteString(escapeTelegramMarkdown(key))
			b.WriteString("](")
			b.WriteString(escapeTelegramMarkdown(msg.Links[key]))
			b.WriteString(")\n")
		}
	}

	return b.String()
}

// escapeTelegramMarkdown escapes special characters for Telegram MarkdownV2.
func escapeTelegramMarkdown(s string) string {
	// Characters that must be escaped in MarkdownV2:
	// _ * [ ] ( ) ~ ` > # + - = | { } . !
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

func sortedFieldKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
