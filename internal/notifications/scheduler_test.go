package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type recordSender struct {
	events []NotificationEvent
}

func (r *recordSender) Send(ctx context.Context, tenantID string, event NotificationEvent, msg NotificationMessage) error {
	r.events = append(r.events, event)
	_ = ctx
	_ = tenantID
	_ = msg
	return nil
}

func TestApprovalExpirySchedulerRun(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	sender := &recordSender{}

	storeMock.On("TryAdvisoryLock", mock.Anything, mock.Anything).Return(true, nil)
	storeMock.On("ReleaseAdvisoryLock", mock.Anything, mock.Anything).Return(nil)
	storeMock.On("ListApprovalsExpiring", mock.Anything, mock.Anything, mock.Anything).
		Return([]models.ApprovalRequest{
			{ApprovalRequestID: "ar1", TenantID: "t1", ActionType: "TYPE", Risk: "HIGH", ExpiresAt: time.Now().Add(2 * time.Minute)},
		}, nil)
	storeMock.On("ListApprovalsExpired", mock.Anything, mock.Anything).
		Return([]models.ApprovalRequest{
			{ApprovalRequestID: "ar2", TenantID: "t1", ActionType: "TYPE", Risk: "HIGH", ExpiresAt: time.Now().Add(-time.Minute)},
		}, nil)
	storeMock.On("MarkApprovalExpired", mock.Anything, "t1", "ar2", mock.Anything).Return(nil)

	scheduler := &ApprovalExpiryScheduler{
		Store:    storeMock,
		Sender:   sender,
		Interval: time.Minute,
		Window:   5 * time.Minute,
		LockKey:  42,
	}

	scheduler.run(context.Background())
	require.Len(t, sender.events, 2)
}

func TestApprovalExpirySchedulerNoLock(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("TryAdvisoryLock", mock.Anything, mock.Anything).Return(false, nil)

	scheduler := &ApprovalExpiryScheduler{
		Store:    storeMock,
		Sender:   &recordSender{},
		Interval: time.Minute,
		Window:   5 * time.Minute,
		LockKey:  42,
	}

	scheduler.run(context.Background())
}
