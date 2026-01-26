package notifications

import (
	"context"
	"errors"
	"fmt"
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
