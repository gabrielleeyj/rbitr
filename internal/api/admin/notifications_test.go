package admin

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleNotificationConfigGet(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not found",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetNotificationConfig", context.Background(), "t1").
					Return(models.NotificationConfig{}, store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetNotificationConfig", context.Background(), "t1").
					Return(models.NotificationConfig{
						TenantID:              "t1",
						SlackWebhookEnabled:   true,
						SlackWebhookSecretRef: "env://SLACK_WEBHOOK",
						SlackBotEnabled:       true,
						SlackBotSecretRef:     "env://SLACK_BOT",
						EmailEnabled:          true,
						EmailSecretRef:        "env://EMAIL",
					}, nil)
			},
			expectedCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet,
				nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleNotificationConfigGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleNotificationConfigUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		body         any
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "invalid body",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         "not-json",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "missing slack channel",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         NotificationConfigRequest{SlackBotEnabled: true},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "missing email provider",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         NotificationConfigRequest{EmailEnabled: true},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body: NotificationConfigRequest{
				SlackWebhookEnabled:    true,
				SlackBotEnabled:        true,
				SlackBotDefaultChannel: "C123",
				EmailEnabled:           true,
				EmailProvider:          "ses",
				EmailFrom:              "alerts@example.com",
				EmailRegion:            "us-east-1",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetNotificationConfig", context.Background(), "t1").
					Return(models.NotificationConfig{
						TenantID:              "t1",
						SlackWebhookSecretRef: "env://SLACK_WEBHOOK",
						SlackBotSecretRef:     "env://SLACK_BOT",
						EmailSecretRef:        "env://EMAIL",
					}, nil)
				storeMock.On("UpsertNotificationConfig", context.Background(), mock.MatchedBy(func(cfg models.NotificationConfig) bool {
					return cfg.TenantID == "t1" && cfg.SlackWebhookEnabled && cfg.SlackWebhookSecretRef != ""
				})).Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			var bodyReader io.Reader
			if tc.body != nil {
				bodyReader = testhelpers.MakeBody(tc.body)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				bodyReader,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleNotificationConfigUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleNotificationSecretRefs(t *testing.T) {
	cases := []struct {
		name         string
		handler      func(Dependencies, *echo.Context) error
		adminKey     string
		scopes       []string
		body         any
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "slack invalid ref",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleNotificationSlackSecretRefSet(c) },
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         SecretRefRequest{SecretRef: "bad://ref"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "email invalid ref",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleNotificationEmailSecretRefSet(c) },
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         SecretRefRequest{SecretRef: "bad://ref"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "slack success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleNotificationSlackSecretRefSet(c) },
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     SecretRefRequest{SecretRef: "env://SLACK_WEBHOOK"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetNotificationConfig", context.Background(), "t1").
					Return(models.NotificationConfig{TenantID: "t1"}, nil)
				storeMock.On("UpsertNotificationConfig", context.Background(), mock.Anything).
					Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "email success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleNotificationEmailSecretRefSet(c) },
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     SecretRefRequest{SecretRef: "env://EMAIL_SECRET"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetNotificationConfig", context.Background(), "t1").
					Return(models.NotificationConfig{TenantID: "t1"}, nil)
				storeMock.On("UpsertNotificationConfig", context.Background(), mock.Anything).
					Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
				Return(modelsAdminKey(tc.scopes), nil)
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				testhelpers.MakeBody(tc.body),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := tc.handler(deps, ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleMailingListCRUD(t *testing.T) {
	cases := []struct {
		name         string
		handler      func(Dependencies, *echo.Context) error
		method       string
		body         any
		params       testhelpers.Params
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "list unauthorized",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleMailingListsList(c) },
			method:       http.MethodGet,
			params:       testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "list success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleMailingListsList(c) },
			method:   http.MethodGet,
			params:   testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListMailingLists", context.Background(), "t1").
					Return([]models.MailingList{}, nil)
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "create invalid email",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleMailingListCreate(c) },
			method:       http.MethodPost,
			params:       testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         MailingListRequest{Name: "Security", Members: []string{"bad-email"}},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "create success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleMailingListCreate(c) },
			method:   http.MethodPost,
			params:   testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     MailingListRequest{Name: "Security", Members: []string{"a@example.com"}},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("CreateMailingList", context.Background(), mock.Anything, []string{"a@example.com"}).
					Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:     "update success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleMailingListUpdate(c) },
			method:   http.MethodPut,
			params:   testhelpers.Params{Names: []string{"tenant_id", "mailing_list_id"}, Values: []string{"t1", "ml1"}},
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     MailingListRequest{Name: "Security", Members: []string{"b@example.com"}},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetMailingList", context.Background(), "t1", "ml1").
					Return(models.MailingList{MailingListID: "ml1", TenantID: "t1"}, nil)
				storeMock.On("UpdateMailingList", context.Background(), mock.Anything, []string{"b@example.com"}).
					Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "delete success",
			handler:  func(d Dependencies, c *echo.Context) error { return d.handleMailingListDelete(c) },
			method:   http.MethodDelete,
			params:   testhelpers.Params{Names: []string{"tenant_id", "mailing_list_id"}, Values: []string{"t1", "ml1"}},
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetMailingList", context.Background(), "t1", "ml1").
					Return(models.MailingList{MailingListID: "ml1", TenantID: "t1", Name: "Security"}, nil)
				storeMock.On("DeleteMailingList", context.Background(), "t1", "ml1").
					Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).
					Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			if tc.adminKey != "" {
				storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
					Return(modelsAdminKey(tc.scopes), nil)
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			var bodyReader io.Reader
			if tc.body != nil {
				bodyReader = testhelpers.MakeBody(tc.body)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(tc.method, bodyReader, tc.params)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := tc.handler(deps, ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleNotificationTestEndpoints(t *testing.T) {
	cases := []struct {
		name         string
		handler      func(Dependencies, *echo.Context) error
		adminKey     string
		scopes       []string
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "slack not configured",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleNotificationTestSlack(c) },
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			expectedCode: http.StatusNotImplemented,
		},
		{
			name:         "email not implemented",
			handler:      func(d Dependencies, c *echo.Context) error { return d.handleNotificationTestEmail(c) },
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			expectedCode: http.StatusNotImplemented,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
				Return(modelsAdminKey(tc.scopes), nil)

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := tc.handler(deps, ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}
