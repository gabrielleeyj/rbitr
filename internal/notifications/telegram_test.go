package notifications

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTelegramNotifier_Name(t *testing.T) {
	n := NewTelegramNotifier("token", "123")
	require.Equal(t, TelegramChannel, n.Name())
}

func TestTelegramNotifier_Send_MissingToken(t *testing.T) {
	n := NewTelegramNotifier("", "123")
	err := n.Send(context.Background(), NotificationMessage{Title: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bot token not configured")
}

func TestTelegramNotifier_Send_MissingChatID(t *testing.T) {
	n := NewTelegramNotifier("token", "")
	err := n.Send(context.Background(), NotificationMessage{Title: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chat_id not configured")
}

func TestBuildTelegramText(t *testing.T) {
	msg := NotificationMessage{
		Title:  "Alert",
		Body:   "Test body",
		Fields: map[string]string{"Key": "Value"},
	}
	text := buildTelegramText(msg)
	require.Contains(t, text, "Alert")
	require.Contains(t, text, "Test body")
	require.Contains(t, text, "Key")
	require.Contains(t, text, "Value")
}

func TestBuildTelegramText_WithLinks(t *testing.T) {
	msg := NotificationMessage{
		Title: "Alert",
		Links: map[string]string{"Dashboard": "https://example.com"},
	}
	text := buildTelegramText(msg)
	require.Contains(t, text, "Dashboard")
	require.Contains(t, text, "example\\.com")
}

func TestEscapeTelegramMarkdown(t *testing.T) {
	input := "Hello_World *bold* [link](url)"
	escaped := escapeTelegramMarkdown(input)
	require.Contains(t, escaped, "\\_")
	require.Contains(t, escaped, "\\*")
	require.Contains(t, escaped, "\\[")
	require.Contains(t, escaped, "\\]")
	require.Contains(t, escaped, "\\(")
	require.Contains(t, escaped, "\\)")
}

func TestEscapeTelegramMarkdown_AllSpecialChars(t *testing.T) {
	specials := "_*[]()~`>#+-=|{}.!"
	escaped := escapeTelegramMarkdown(specials)
	for _, ch := range specials {
		require.Contains(t, escaped, "\\"+string(ch))
	}
}
