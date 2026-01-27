package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

type Service struct {
	Store    store.StoreAPI
	Resolver SecretResolver
	Cooldown time.Duration
	Metrics  *telemetry.Metrics
}

type NotificationSender interface {
	Send(ctx context.Context, tenantID string, event NotificationEvent, msg NotificationMessage) error
}

func NewService(st store.StoreAPI, resolver SecretResolver, cooldown time.Duration, metrics *telemetry.Metrics) *Service {
	return &Service{
		Store:    st,
		Resolver: resolver,
		Cooldown: cooldown,
		Metrics:  metrics,
	}
}

func (s *Service) Send(ctx context.Context, tenantID string, event NotificationEvent, msg NotificationMessage) error {
	config, err := s.Store.GetNotificationConfig(ctx, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	if !eventEnabled(config, event.EventType) {
		return nil
	}

	var errs []error
	if config.SlackWebhookEnabled && config.SlackWebhookSecretRef != "" {
		webhookURL, err := s.ResolveSecret(ctx, config.SlackWebhookSecretRef)
		if err != nil {
			errs = append(errs, err)
		} else {
			engine := NewEngine(s.Store, map[string]Notifier{
				SlackWebhookChannel: NewSlackWebhookNotifier(webhookURL, config.SlackWebhookDefaultChannel),
			}, s.Cooldown, s.Metrics)
			if err := engine.Send(ctx, SlackWebhookChannel, event, msg); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if config.SlackBotEnabled && config.SlackBotSecretRef != "" && config.SlackBotDefaultChannel != "" {
		token, err := s.ResolveSecret(ctx, config.SlackBotSecretRef)
		if err != nil {
			errs = append(errs, err)
		} else {
			engine := NewEngine(s.Store, map[string]Notifier{
				SlackBotChannel: NewSlackBotNotifier(token, config.SlackBotDefaultChannel, nil, ""),
			}, s.Cooldown, s.Metrics)
			if err := engine.Send(ctx, SlackBotChannel, event, msg); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if config.EmailEnabled {
		emailErr := s.sendEmail(ctx, config, event, msg)
		if emailErr != nil {
			errs = append(errs, emailErr)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Service) ResolveSecret(ctx context.Context, ref string) (string, error) {
	if s.Resolver == nil {
		return "", fmt.Errorf("secret resolver not configured")
	}
	return s.Resolver.Resolve(ctx, ref)
}

func eventEnabled(config models.NotificationConfig, eventType string) bool {
	switch eventType {
	case EventApprovalExpiring, EventApprovalExpired:
		return config.NotifyApprovalExpiring
	case "SECURITY.TOKEN_ABUSE":
		return config.NotifyTokenAbuse
	case "POLICY.INVALID_OUTPUT", "POLICY.EVAL_ERROR":
		return config.NotifyPolicyInvalid
	default:
		return true
	}
}

func (s *Service) sendEmail(ctx context.Context, config models.NotificationConfig, event NotificationEvent, msg NotificationMessage) error {
	if config.EmailProvider == "" || config.EmailFrom == "" {
		return fmt.Errorf("email provider or from address missing")
	}
	if config.EmailDefaultMailingListID == "" {
		return fmt.Errorf("email default mailing list missing")
	}

	members, err := s.Store.ListMailingListMembers(ctx, config.EmailDefaultMailingListID)
	if err != nil {
		return err
	}
	var recipients []string
	for _, member := range members {
		if member.Email != "" {
			recipients = append(recipients, member.Email)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email mailing list has no members")
	}

	secretValue := ""
	if config.EmailSecretRef != "" {
		secretValue, err = s.ResolveSecret(ctx, config.EmailSecretRef)
		if err != nil {
			return err
		}
	}

	provider := strings.ToLower(config.EmailProvider)
	var sender EmailSender
	switch provider {
	case "ses":
		sender, err = NewSESSender(ctx, config.EmailRegion, secretValue)
	case "sendgrid":
		sender, err = NewSendGridSender(secretValue)
	case "mailgun":
		sender, err = NewMailgunSender(secretValue, config.EmailDomain)
	default:
		return fmt.Errorf("unsupported email provider")
	}
	if err != nil {
		return err
	}

	engine := NewEngine(s.Store, map[string]Notifier{
		EmailChannel: NewEmailNotifier(sender, config.EmailFrom, recipients),
	}, s.Cooldown, s.Metrics)
	return engine.Send(ctx, EmailChannel, event, msg)
}
