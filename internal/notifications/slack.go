package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/slack-go/slack"
)

const (
	SlackWebhookChannel = "slack_webhook"
	SlackBotChannel     = "slack_bot"
)

type SlackWebhookNotifier struct {
	webhookURL     string
	defaultChannel string
	client         *http.Client
}

func NewSlackWebhookNotifier(webhookURL, defaultChannel string) *SlackWebhookNotifier {
	return &SlackWebhookNotifier{
		webhookURL:     webhookURL,
		defaultChannel: defaultChannel,
		client:         http.DefaultClient,
	}
}

func (n *SlackWebhookNotifier) Name() string {
	return SlackWebhookChannel
}

func (n *SlackWebhookNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	if n.webhookURL == "" {
		return fmt.Errorf("slack webhook not configured")
	}

	payload := slack.WebhookMessage{
		Text:    buildSlackText(msg),
		Blocks:  &slack.Blocks{BlockSet: buildSlackBlocks(msg)},
		Channel: n.defaultChannel,
	}
	body, err := json.Marshal(&payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook failed: status %d", resp.StatusCode)
	}
	return nil
}

type SlackBotNotifier struct {
	client         *slack.Client
	defaultChannel string
}

func NewSlackBotNotifier(token, defaultChannel string, httpClient *http.Client, apiURL string) *SlackBotNotifier {
	options := []slack.Option{}
	if httpClient != nil {
		options = append(options, slack.OptionHTTPClient(httpClient))
	}
	if apiURL != "" {
		options = append(options, slack.OptionAPIURL(apiURL))
	}
	return &SlackBotNotifier{
		client:         slack.New(token, options...),
		defaultChannel: defaultChannel,
	}
}

func (n *SlackBotNotifier) Name() string {
	return SlackBotChannel
}

func (n *SlackBotNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	if n.client == nil {
		return fmt.Errorf("slack bot not configured")
	}
	_, _, err := n.client.PostMessageContext(
		ctx,
		n.defaultChannel,
		slack.MsgOptionText(buildSlackText(msg), false),
		slack.MsgOptionBlocks(buildSlackBlocks(msg)...),
	)
	return err
}

func buildSlackText(msg NotificationMessage) string {
	var b strings.Builder
	if msg.Title != "" {
		b.WriteString(msg.Title)
	}
	if msg.Body != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(msg.Body)
	}
	if len(msg.Fields) > 0 {
		for _, key := range sortedKeys(msg.Fields) {
			b.WriteString("\n")
			b.WriteString("*")
			b.WriteString(key)
			b.WriteString("*: ")
			b.WriteString(msg.Fields[key])
		}
	}
	if len(msg.Links) > 0 {
		b.WriteString("\n")
		for _, key := range sortedKeys(msg.Links) {
			b.WriteString("\n")
			b.WriteString("• ")
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(msg.Links[key])
		}
	}
	return b.String()
}

func buildSlackBlocks(msg NotificationMessage) []slack.Block {
	blocks := make([]slack.Block, 0, 3)
	if msg.Title != "" {
		blocks = append(blocks, slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", msg.Title, false, false)))
	}

	fields := make([]*slack.TextBlockObject, 0, len(msg.Fields))
	for _, key := range sortedKeys(msg.Fields) {
		value := fmt.Sprintf("*%s*\n%s", key, msg.Fields[key])
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", value, false, false))
	}
	if len(fields) > 0 || msg.Body != "" {
		text := slack.NewTextBlockObject("mrkdwn", msg.Body, false, false)
		blocks = append(blocks, slack.NewSectionBlock(text, fields, nil))
	}

	if len(msg.Links) > 0 {
		var b strings.Builder
		for _, key := range sortedKeys(msg.Links) {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("<%s|%s>", msg.Links[key], key))
		}
		linkText := slack.NewTextBlockObject("mrkdwn", b.String(), false, false)
		blocks = append(blocks, slack.NewSectionBlock(linkText, nil, nil))
	}

	return blocks
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
