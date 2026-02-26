package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type stubNotifier struct {
	name  string
	err   error
	calls int
}

func (s *stubNotifier) Send(ctx context.Context, msg NotificationMessage) error {
	s.calls++
	_ = ctx
	_ = msg
	return s.err
}

func (s *stubNotifier) Name() string {
	return s.name
}

func TestEngineSendSuccess(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	event := NotificationEvent{
		TenantID:   "t1",
		EventType:  "APPROVAL.EXPIRING",
		Severity:   "WARN",
		ResourceID: "ar1",
	}
	channel := "slack"
	key := dedupKey(event, channel)

	storeMock.On("GetNotificationSuppression", context.Background(), key).
		Return(models.NotificationSuppression{}, store.ErrNotFound)
	storeMock.On("UpsertNotificationSuppression", context.Background(), mock.MatchedBy(func(s *models.NotificationSuppression) bool {
		return s != nil && s.DedupKey == key && s.LastSentAt != nil && s.SuppressedUntil != nil
	})).Return(nil)

	notifier := &stubNotifier{name: "slack"}
	engine := NewEngine(storeMock, map[string]Notifier{channel: notifier}, time.Minute, nil)
	err := engine.Send(context.Background(), channel, event, NotificationMessage{Title: "t"})
	require.NoError(t, err)
	require.Equal(t, 1, notifier.calls)
}

func TestEngineSendSuppressed(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	now := time.Now().UTC()
	event := NotificationEvent{
		TenantID:   "t1",
		EventType:  "APPROVAL.EXPIRING",
		Severity:   "WARN",
		ResourceID: "ar1",
	}
	channel := "slack"
	key := dedupKey(event, channel)

	storeMock.On("GetNotificationSuppression", context.Background(), key).
		Return(models.NotificationSuppression{
			DedupKey:        key,
			TenantID:        "t1",
			Channel:         channel,
			EventType:       event.EventType,
			Severity:        event.Severity,
			LastSentAt:      ptrTime(now),
			SuppressedUntil: ptrTime(now.Add(5 * time.Minute)),
			SuppressedCount: 1,
		}, nil)
	storeMock.On("UpsertNotificationSuppression", context.Background(), mock.MatchedBy(func(s *models.NotificationSuppression) bool {
		return s != nil && s.DedupKey == key && s.SuppressedCount == 2
	})).Return(nil)

	notifier := &stubNotifier{name: "slack"}
	engine := NewEngine(storeMock, map[string]Notifier{channel: notifier}, 10*time.Minute, nil)
	err := engine.Send(context.Background(), channel, event, NotificationMessage{Title: "t"})
	require.NoError(t, err)
	require.Equal(t, 0, notifier.calls)
}

func TestEngineSendNotifierError(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	event := NotificationEvent{
		TenantID:   "t1",
		EventType:  "POLICY.INVALID_OUTPUT",
		Severity:   "CRITICAL",
		ResourceID: "p_v1",
	}
	channel := "slack"
	key := dedupKey(event, channel)

	storeMock.On("GetNotificationSuppression", context.Background(), key).
		Return(models.NotificationSuppression{}, store.ErrNotFound)
	storeMock.On("UpsertNotificationSuppression", context.Background(), mock.Anything).
		Return(nil)

	notifier := &stubNotifier{name: "slack", err: errors.New("boom")}
	engine := NewEngine(storeMock, map[string]Notifier{channel: notifier}, time.Minute, nil)
	err := engine.Send(context.Background(), channel, event, NotificationMessage{Title: "t"})
	require.Error(t, err)
	require.Equal(t, 1, notifier.calls)
}

func TestEngineMissingNotifier(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	engine := NewEngine(storeMock, map[string]Notifier{}, time.Minute, nil)
	err := engine.Send(context.Background(), "slack", NotificationEvent{TenantID: "t1"}, NotificationMessage{})
	require.Error(t, err)
}
