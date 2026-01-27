package retention

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/store"
)

func TestAuditRetentionSchedulerRun(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("TryAdvisoryLock", mock.Anything, mock.Anything).Return(true, nil)
	storeMock.On("ReleaseAdvisoryLock", mock.Anything, mock.Anything).Return(nil)
	storeMock.On("GetAuditRetentionDays", mock.Anything).Return(365, nil)
	storeMock.On("DeleteAuditEventsBefore", mock.Anything, mock.Anything).Return(int64(3), nil)

	scheduler := NewAuditRetentionScheduler(storeMock, time.Hour)
	scheduler.run(context.Background())

	require.True(t, storeMock.AssertCalled(t, "DeleteAuditEventsBefore", mock.Anything, mock.Anything))
}
