package notifications

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWhatsAppNotifier_Name(t *testing.T) {
	n := NewWhatsAppNotifier("token", "phone-id", "+1234567890")
	require.Equal(t, WhatsAppChannel, n.Name())
}

func TestWhatsAppNotifier_Send_MissingToken(t *testing.T) {
	n := NewWhatsAppNotifier("", "phone-id", "+123")
	err := n.Send(context.Background(), NotificationMessage{Title: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "access token not configured")
}

func TestWhatsAppNotifier_Send_MissingPhoneID(t *testing.T) {
	n := NewWhatsAppNotifier("token", "", "+123")
	err := n.Send(context.Background(), NotificationMessage{Title: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "phone number id not configured")
}

func TestWhatsAppNotifier_Send_MissingRecipient(t *testing.T) {
	n := NewWhatsAppNotifier("token", "phone-id", "")
	err := n.Send(context.Background(), NotificationMessage{Title: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "default recipient not configured")
}

func TestBuildWhatsAppText(t *testing.T) {
	msg := NotificationMessage{
		Title:  "Alert",
		Body:   "Test body",
		Fields: map[string]string{"Key": "Value"},
		Links:  map[string]string{"Dashboard": "https://example.com"},
	}
	text := buildWhatsAppText(msg)
	require.Contains(t, text, "*Alert*")
	require.Contains(t, text, "Test body")
	require.Contains(t, text, "*Key*: Value")
	require.Contains(t, text, "Dashboard: https://example.com")
}

func TestBuildWhatsAppText_TitleOnly(t *testing.T) {
	msg := NotificationMessage{Title: "Hello"}
	text := buildWhatsAppText(msg)
	require.Equal(t, "*Hello*\n", text)
}

func TestBuildWhatsAppText_Empty(t *testing.T) {
	msg := NotificationMessage{}
	text := buildWhatsAppText(msg)
	require.Empty(t, text)
}
