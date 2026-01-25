package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSlackWebhookNotifier(t *testing.T) {
	var got slack.WebhookMessage
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode webhook: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	notifier := NewSlackWebhookNotifier("http://example/webhook", "C01")
	notifier.client = client
	msg := NotificationMessage{
		Title: "Approval expiring soon",
		Body:  "Approval ar1 expires soon",
		Fields: map[string]string{
			"Tenant": "t1",
		},
		Links: map[string]string{
			"View approval": "https://example/approval",
		},
	}
	if err := notifier.Send(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text == "" {
		t.Fatalf("expected webhook text")
	}
	if got.Channel != "C01" {
		t.Fatalf("expected channel C01 got %q", got.Channel)
	}
}

func TestSlackBotNotifier(t *testing.T) {
	var gotBody string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"channel":"C01","ts":"1"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	notifier := NewSlackBotNotifier("token", "C01", client, "http://slack.local/")
	msg := NotificationMessage{Title: "Policy invalid", Body: "Check policy"}
	if err := notifier.Send(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if values.Get("channel") != "C01" {
		t.Fatalf("expected channel C01 got %q", values.Get("channel"))
	}
}

func TestSlackBlocksAndText(t *testing.T) {
	msg := NotificationMessage{
		Title: "Header",
		Body:  "Body",
		Fields: map[string]string{
			"B": "2",
			"A": "1",
		},
		Links: map[string]string{
			"View": "https://example",
		},
	}
	text := buildSlackText(msg)
	if !strings.Contains(text, "Header") || !strings.Contains(text, "*A*: 1") {
		t.Fatalf("unexpected text: %s", text)
	}
	blocks := buildSlackBlocks(msg)
	if len(blocks) == 0 {
		t.Fatalf("expected blocks")
	}
}
