package notifications

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

type fakeResolver struct {
	value string
	err   error
}

func (f fakeResolver) Resolve(ctx context.Context, ref string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.value, nil
}

func TestNewService(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	resolver := fakeResolver{value: "ok"}
	service := NewService(st, resolver, 10, nil)
	require.Equal(t, st, service.Store)
	require.Equal(t, resolver, service.Resolver)
	require.Equal(t, 10, int(service.Cooldown))
}

func TestResolveSecretMissingResolver(t *testing.T) {
	service := &Service{}
	_, err := service.ResolveSecret(context.Background(), "env://KEY")
	require.Error(t, err)
}

func TestSendEventDisabled(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	st.On("GetNotificationConfig", context.Background(), "t1").
		Return(models.NotificationConfig{
			TenantID:               "t1",
			NotifyApprovalExpiring: false,
		}, nil)

	service := NewService(st, fakeResolver{value: "ignored"}, 0, nil)
	err := service.Send(context.Background(), "t1", NotificationEvent{
		TenantID:   "t1",
		EventType:  EventApprovalExpiring,
		Severity:   SeverityWarn,
		ResourceID: "ar1",
	}, NotificationMessage{Title: "x"})
	require.NoError(t, err)
	st.AssertExpectations(t)
}

func TestSendEmailMissingConfig(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	st.On("GetNotificationConfig", context.Background(), "t1").
		Return(models.NotificationConfig{
			TenantID:               "t1",
			EmailEnabled:           true,
			EmailProvider:          "",
			EmailFrom:              "",
			EmailSecretRef:         "",
			NotifyApprovalExpiring: true,
		}, nil)

	service := NewService(st, fakeResolver{}, 0, nil)
	err := service.Send(context.Background(), "t1", NotificationEvent{
		TenantID:   "t1",
		EventType:  EventApprovalExpiring,
		Severity:   SeverityWarn,
		ResourceID: "ar1",
	}, NotificationMessage{Title: "x"})
	require.Error(t, err)
}

func TestSendEmailMissingMailingList(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	st.On("GetNotificationConfig", context.Background(), "t1").
		Return(models.NotificationConfig{
			TenantID:               "t1",
			EmailEnabled:           true,
			EmailProvider:          "sendgrid",
			EmailFrom:              "alerts@example.com",
			NotifyApprovalExpiring: true,
		}, nil)

	service := NewService(st, fakeResolver{}, 0, nil)
	err := service.Send(context.Background(), "t1", NotificationEvent{
		TenantID:   "t1",
		EventType:  EventApprovalExpiring,
		Severity:   SeverityWarn,
		ResourceID: "ar1",
	}, NotificationMessage{Title: "x"})
	require.Error(t, err)
}

func TestSendEmailEmptyRecipients(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	st.On("GetNotificationConfig", context.Background(), "t1").
		Return(models.NotificationConfig{
			TenantID:                  "t1",
			EmailEnabled:              true,
			EmailProvider:             "sendgrid",
			EmailFrom:                 "alerts@example.com",
			EmailDefaultMailingListID: "ml1",
			NotifyApprovalExpiring:    true,
		}, nil)
	st.On("ListMailingListMembers", context.Background(), "ml1").
		Return([]models.MailingListMember{}, nil)

	service := NewService(st, fakeResolver{}, 0, nil)
	err := service.Send(context.Background(), "t1", NotificationEvent{
		TenantID:   "t1",
		EventType:  EventApprovalExpiring,
		Severity:   SeverityWarn,
		ResourceID: "ar1",
	}, NotificationMessage{Title: "x"})
	require.Error(t, err)
	st.AssertExpectations(t)
}

func TestSendEmailMissingSecret(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	st.On("GetNotificationConfig", context.Background(), "t1").
		Return(models.NotificationConfig{
			TenantID:                  "t1",
			EmailEnabled:              true,
			EmailProvider:             "sendgrid",
			EmailFrom:                 "alerts@example.com",
			EmailDefaultMailingListID: "ml1",
			NotifyApprovalExpiring:    true,
		}, nil)
	st.On("ListMailingListMembers", context.Background(), "ml1").
		Return([]models.MailingListMember{{MailingListID: "ml1", Email: "a@example.com"}}, nil)

	service := NewService(st, fakeResolver{}, 0, nil)
	err := service.Send(context.Background(), "t1", NotificationEvent{
		TenantID:   "t1",
		EventType:  EventApprovalExpiring,
		Severity:   SeverityWarn,
		ResourceID: "ar1",
	}, NotificationMessage{Title: "x"})
	require.Error(t, err)
	st.AssertExpectations(t)
}

func TestResolveSecretError(t *testing.T) {
	st := store.NewMockStoreAPI(t)
	service := NewService(st, fakeResolver{err: errors.New("fail")}, 0, nil)
	_, err := service.ResolveSecret(context.Background(), "env://KEY")
	require.Error(t, err)
}
