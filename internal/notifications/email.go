package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/mailgun/mailgun-go/v5"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

const EmailChannel = "email"

type EmailSender interface {
	Send(ctx context.Context, from string, to []string, subject, body string) error
}

type emailNotifier struct {
	sender EmailSender
	from   string
	to     []string
}

func NewEmailNotifier(sender EmailSender, from string, to []string) Notifier {
	return &emailNotifier{
		sender: sender,
		from:   from,
		to:     to,
	}
}

func (n *emailNotifier) Name() string {
	return EmailChannel
}

func (n *emailNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	if n.sender == nil {
		return errors.New("email sender not configured")
	}
	if n.from == "" {
		return errors.New("email from address required")
	}
	if len(n.to) == 0 {
		return errors.New("email recipients missing")
	}
	subject := buildEmailSubject(msg)
	body := buildEmailBody(msg)
	return n.sender.Send(ctx, n.from, n.to, subject, body)
}

type sesCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Region          string `json:"region"`
}

type sesSender struct {
	client *sesv2.Client
}

func NewSESSender(ctx context.Context, region, secretValue string) (EmailSender, error) {
	creds, err := parseSESCredentials(secretValue)
	if err != nil {
		return nil, err
	}
	finalRegion := region
	if finalRegion == "" && creds != nil {
		finalRegion = creds.Region
	}
	if finalRegion == "" {
		return nil, errors.New("ses region required")
	}
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(finalRegion),
	}
	if creds != nil && creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
		))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &sesSender{client: sesv2.NewFromConfig(cfg)}, nil
}

func (s *sesSender) Send(ctx context.Context, from string, to []string, subject, body string) error {
	if s.client == nil {
		return errors.New("ses client not configured")
	}
	if len(to) == 0 {
		return errors.New("ses recipients missing")
	}
	_, err := s.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &from,
		Destination: &types.Destination{
			ToAddresses: to,
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data: &subject,
				},
				Body: &types.Body{
					Text: &types.Content{
						Data: &body,
					},
				},
			},
		},
	})
	return err
}

type sendGridSender struct {
	client *sendgrid.Client
}

func NewSendGridSender(apiKey string) (EmailSender, error) {
	if apiKey == "" {
		return nil, errors.New("sendgrid api key required")
	}
	return &sendGridSender{client: sendgrid.NewSendClient(apiKey)}, nil
}

func (s *sendGridSender) Send(ctx context.Context, from string, to []string, subject, body string) error {
	if s.client == nil {
		return errors.New("sendgrid client not configured")
	}
	if len(to) == 0 {
		return errors.New("sendgrid recipients missing")
	}
	message := mail.NewV3MailInit(mail.NewEmail("", from), subject, nil, mail.NewContent("text/plain", body))
	personalization := mail.NewPersonalization()
	for _, addr := range to {
		personalization.AddTos(mail.NewEmail("", addr))
	}
	message.AddPersonalizations(personalization)
	resp, err := s.client.SendWithContext(ctx, message)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 { //nolint:mnd // just need to check status codes above 300.
		return fmt.Errorf("sendgrid status %d", resp.StatusCode)
	}
	return nil
}

type mailgunSender struct {
	client mailgun.Mailgun
	domain string
}

func NewMailgunSender(apiKey, domain string) (EmailSender, error) {
	if apiKey == "" {
		return nil, errors.New("mailgun api key required")
	}
	if domain == "" {
		return nil, errors.New("mailgun domain required")
	}
	return &mailgunSender{client: mailgun.NewMailgun(apiKey), domain: domain}, nil
}

func (m *mailgunSender) Send(ctx context.Context, from string, to []string, subject, body string) error {
	if m.client == nil {
		return errors.New("mailgun client not configured")
	}
	if len(to) == 0 {
		return errors.New("mailgun recipients missing")
	}
	message := mailgun.NewMessage(m.domain, from, subject, body, to...)
	_, err := m.client.Send(ctx, message)
	return err
}

//nolint:nilnil,mnd // empty credential value is treated as "not configured" rather than an error.
func parseSESCredentials(value string) (*sesCredentials, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var creds sesCredentials
		if err := json.Unmarshal([]byte(trimmed), &creds); err != nil {
			return nil, errors.New("invalid ses credentials json")
		}
		if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
			return nil, errors.New("ses credentials missing access_key_id or secret_access_key")
		}
		return &creds, nil
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, errors.New("ses credentials invalid")
		}
		return &sesCredentials{AccessKeyID: parts[0], SecretAccessKey: parts[1]}, nil
	}
	return nil, errors.New("ses credentials invalid")
}

func buildEmailSubject(msg NotificationMessage) string {
	if msg.Title != "" {
		return msg.Title
	}
	return "rbitr notification"
}

func buildEmailBody(msg NotificationMessage) string {
	var b strings.Builder
	if msg.Title != "" {
		b.WriteString(msg.Title)
		b.WriteString("\n\n")
	}
	if msg.Body != "" {
		b.WriteString(msg.Body)
		b.WriteString("\n\n")
	}
	if len(msg.Fields) > 0 {
		for _, key := range sortedKeys(msg.Fields) {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(msg.Fields[key])
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(msg.Links) > 0 {
		for _, key := range sortedKeys(msg.Links) {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(msg.Links[key])
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}
