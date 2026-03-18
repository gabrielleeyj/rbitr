package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	WhatsAppChannel    = "whatsapp"
	whatsappAPIBaseURL = "https://graph.facebook.com/v21.0"
)

type WhatsAppNotifier struct {
	accessToken      string
	phoneNumberID    string
	defaultRecipient string
	client           *http.Client
}

func NewWhatsAppNotifier(accessToken, phoneNumberID, defaultRecipient string) *WhatsAppNotifier {
	return &WhatsAppNotifier{
		accessToken:      accessToken,
		phoneNumberID:    phoneNumberID,
		defaultRecipient: defaultRecipient,
		client:           http.DefaultClient,
	}
}

func (n *WhatsAppNotifier) Name() string {
	return WhatsAppChannel
}

func (n *WhatsAppNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	if n.accessToken == "" {
		return errors.New("whatsapp access token not configured")
	}
	if n.phoneNumberID == "" {
		return errors.New("whatsapp phone number id not configured")
	}
	if n.defaultRecipient == "" {
		return errors.New("whatsapp default recipient not configured")
	}

	text := buildWhatsAppText(msg)
	payload := whatsappMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               n.defaultRecipient,
		Type:             "text",
		Text: whatsappTextPayload{
			PreviewURL: false,
			Body:       text,
		},
	}

	body, err := json.Marshal(&payload)
	if err != nil {
		return fmt.Errorf("whatsapp marshal failed: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", whatsappAPIBaseURL, n.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("whatsapp request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.accessToken)

	client := n.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp API error: status %d", resp.StatusCode)
	}
	return nil
}

type whatsappMessageRequest struct {
	MessagingProduct string              `json:"messaging_product"`
	RecipientType    string              `json:"recipient_type"`
	To               string              `json:"to"`
	Type             string              `json:"type"`
	Text             whatsappTextPayload `json:"text"`
}

type whatsappTextPayload struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

func buildWhatsAppText(msg NotificationMessage) string {
	var b strings.Builder

	if msg.Title != "" {
		b.WriteString("*")
		b.WriteString(msg.Title)
		b.WriteString("*\n")
	}
	if msg.Body != "" {
		b.WriteString(msg.Body)
		b.WriteString("\n")
	}
	if len(msg.Fields) > 0 {
		b.WriteString("\n")
		keys := sortedFieldKeys(msg.Fields)
		for _, key := range keys {
			b.WriteString("*")
			b.WriteString(key)
			b.WriteString("*: ")
			b.WriteString(msg.Fields[key])
			b.WriteString("\n")
		}
	}
	if len(msg.Links) > 0 {
		b.WriteString("\n")
		keys := sortedFieldKeys(msg.Links)
		for _, key := range keys {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(msg.Links[key])
			b.WriteString("\n")
		}
	}

	return b.String()
}
