package notifications

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

const (
	ResultSent       = "sent"
	ResultFailed     = "failed"
	ResultSuppressed = "suppressed"
)

type Notifier interface {
	Send(ctx context.Context, msg NotificationMessage) error
	Name() string
}

type NotificationEvent struct {
	TenantID   string
	EventType  string
	Severity   string
	ResourceID string
}

type NotificationMessage struct {
	Title  string
	Body   string
	Fields map[string]string
	Links  map[string]string
}

type Engine struct {
	Store     store.StoreAPI
	Notifiers map[string]Notifier
	Cooldown  time.Duration
	Metrics   *telemetry.Metrics
}

func NewEngine(st store.StoreAPI, notifiers map[string]Notifier, cooldown time.Duration, metrics *telemetry.Metrics) *Engine {
	if cooldown == 0 {
		cooldown = 10 * time.Minute
	}
	return &Engine{
		Store:     st,
		Notifiers: notifiers,
		Cooldown:  cooldown,
		Metrics:   metrics,
	}
}

func (e *Engine) Send(ctx context.Context, channel string, event NotificationEvent, msg NotificationMessage) error {
	notifier, ok := e.Notifiers[channel]
	if !ok {
		return fmt.Errorf("notifier not configured: %s", channel)
	}
	dedupKey := dedupKey(event, channel)
	payloadHash := hashMessage(msg)
	now := time.Now().UTC()

	suppression, err := e.Store.GetNotificationSuppression(ctx, dedupKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	var existing models.NotificationSuppression
	if err == nil {
		existing = suppression
	}

	shouldSend := true
	if err == nil {
		if suppression.LastSentAt != nil && now.Sub(*suppression.LastSentAt) < e.Cooldown {
			shouldSend = false
		}
		if suppression.SuppressedUntil != nil && now.Before(*suppression.SuppressedUntil) {
			shouldSend = false
		}
	}

	if !shouldSend {
		next := buildSuppression(event, channel, dedupKey, payloadHash, existing.FirstSeenAt)
		next = mergeSuppression(next, existing)
		next.LastSeenAt = now
		next.SuppressedCount++
		if err := e.Store.UpsertNotificationSuppression(ctx, next); err != nil {
			return err
		}
		if e.Metrics != nil && e.Metrics.NotificationsSuppressedTotal != nil {
			e.Metrics.NotificationsSuppressedTotal.WithLabelValues(channel, event.EventType).Inc()
		}
		return nil
	}

	start := time.Now()
	sendErr := notifier.Send(ctx, msg)
	latency := time.Since(start)

	if sendErr != nil {
		next := buildSuppression(event, channel, dedupKey, payloadHash, existing.FirstSeenAt)
		next = mergeSuppression(next, existing)
		next.LastSeenAt = now
		if err := e.Store.UpsertNotificationSuppression(ctx, next); err != nil {
			return err
		}
		recordNotificationMetrics(e.Metrics, channel, event.EventType, ResultFailed, latency)
		return sendErr
	}

	next := buildSuppression(event, channel, dedupKey, payloadHash, existing.FirstSeenAt)
	next = mergeSuppression(next, existing)
	next.LastSeenAt = now
	next.LastSentAt = &now
	next.SuppressedUntil = ptrTime(now.Add(e.Cooldown))
	if err := e.Store.UpsertNotificationSuppression(ctx, next); err != nil {
		return err
	}
	recordNotificationMetrics(e.Metrics, channel, event.EventType, ResultSent, latency)
	return nil
}

func buildSuppression(event NotificationEvent, channel, dedupKey, payloadHash string, firstSeen time.Time) models.NotificationSuppression {
	if firstSeen.IsZero() {
		firstSeen = time.Now().UTC()
	}
	return models.NotificationSuppression{
		DedupKey:        dedupKey,
		TenantID:        event.TenantID,
		Channel:         channel,
		EventType:       event.EventType,
		ResourceID:      event.ResourceID,
		Severity:        event.Severity,
		FirstSeenAt:     firstSeen,
		LastPayloadHash: payloadHash,
	}
}

func mergeSuppression(next, existing models.NotificationSuppression) models.NotificationSuppression {
	if existing.DedupKey == "" {
		return next
	}
	next.FirstSeenAt = existing.FirstSeenAt
	next.SuppressedCount = existing.SuppressedCount
	next.LastSentAt = existing.LastSentAt
	next.SuppressedUntil = existing.SuppressedUntil
	return next
}

func dedupKey(event NotificationEvent, channel string) string {
	canonical := strings.Join([]string{
		event.TenantID,
		event.EventType,
		event.ResourceID,
		event.Severity,
		channel,
	}, "|")
	return utils.HashString(canonical)
}

func hashMessage(msg NotificationMessage) string {
	var b strings.Builder
	b.WriteString(msg.Title)
	b.WriteString("|")
	b.WriteString(msg.Body)
	writeMap(&b, msg.Fields)
	writeMap(&b, msg.Links)
	return utils.HashString(b.String())
}

func writeMap(b *strings.Builder, values map[string]string) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString("|")
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(values[key])
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func recordNotificationMetrics(metrics *telemetry.Metrics, channel, eventType, result string, latency time.Duration) {
	if metrics == nil {
		return
	}
	if metrics.NotificationsSentTotal != nil {
		metrics.NotificationsSentTotal.WithLabelValues(channel, eventType, result).Inc()
	}
	if metrics.NotificationsLatencyMs != nil {
		metrics.NotificationsLatencyMs.WithLabelValues(channel).Observe(float64(latency.Milliseconds()))
	}
}
