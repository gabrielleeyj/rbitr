package notifications

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSESCredentials(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		expectErr bool
	}{
		{"empty", "", false},
		{"json", `{"access_key_id":"a","secret_access_key":"b","region":"us-east-1"}`, false},
		{"colon", "a:b", false},
		{"bad", "abc", true},
		{"json_missing", `{"access_key_id":""}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSESCredentials(tc.value)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEmailNotifierValidation(t *testing.T) {
	notify := &emailNotifier{}
	require.Error(t, notify.Send(context.Background(), NotificationMessage{Title: "hi"}))

	notify = &emailNotifier{sender: &sendGridSender{}}
	require.Error(t, notify.Send(context.Background(), NotificationMessage{Title: "hi"}))

	notify = &emailNotifier{sender: &sendGridSender{}, from: "a@example.com"}
	require.Error(t, notify.Send(context.Background(), NotificationMessage{Title: "hi"}))
}

func TestEmailBodyAndSubject(t *testing.T) {
	msg := NotificationMessage{
		Title: "Approval expiring soon",
		Body:  "Check approval",
		Fields: map[string]string{
			"Tenant": "t1",
		},
		Links: map[string]string{
			"Open": "https://example.com",
		},
	}
	require.Equal(t, "Approval expiring soon", buildEmailSubject(msg))
	body := buildEmailBody(msg)
	require.Contains(t, body, "Approval expiring soon")
	require.Contains(t, body, "Check approval")
	require.Contains(t, body, "Tenant: t1")
	require.Contains(t, body, "Open: https://example.com")
}

func TestSendersValidate(t *testing.T) {
	mgSender, err := NewMailgunSender("", "mg.example.com")
	require.Error(t, err)
	require.Nil(t, mgSender)

	mgSender, err = NewMailgunSender("key", "")
	require.Error(t, err)
	require.Nil(t, mgSender)

	sgSender, err := NewSendGridSender("")
	require.Error(t, err)
	require.Nil(t, sgSender)

	mg := &mailgunSender{}
	require.Error(t, mg.Send(context.Background(), "from@example.com", []string{"a@example.com"}, "subject", "body"))

	sg := &sendGridSender{}
	require.Error(t, sg.Send(context.Background(), "from@example.com", []string{"a@example.com"}, "subject", "body"))

	ses := &sesSender{}
	require.Error(t, ses.Send(context.Background(), "from@example.com", []string{"a@example.com"}, "subject", "body"))

	sesSender, err := NewSESSender(context.Background(), "", "")
	require.Error(t, err)
	require.Nil(t, sesSender)
}
