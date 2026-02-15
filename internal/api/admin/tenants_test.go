package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleTenantCreate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		body         string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		checkBody    bool
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "missing name",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         `{}`,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     `{"name":"New Tenant"}`,
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("CreateTenant", mock.Anything, mock.MatchedBy(func(id string) bool {
					return len(id) > 2 && id[:2] == "t_"
				}), "New Tenant").Return(nil)
				m.On("CreateTenantKey", mock.Anything, mock.Anything).Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
			checkBody:    true,
		},
		{
			name:     "store error",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     `{"name":"Fail"}`,
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("CreateTenant", mock.Anything, mock.Anything, "Fail").Return(store.ErrAdminWriteLocked)
			},
			expectedCode: http.StatusInternalServerError,
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

			body := tc.body
			if body == "" {
				body = `{}`
			}
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(json.RawMessage(body)),
				testhelpers.Params{},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			_ = deps.handleTenantCreate(ctx)
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.checkBody {
				var resp CreateTenantResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.NotEmpty(t, resp.TenantID)
				require.NotEmpty(t, resp.APIKey)
				require.NotEmpty(t, resp.KeyID)
				require.Equal(t, "New Tenant", resp.Name)
			}
		})
	}
}

func TestHandleTenantKeysList(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedLen  int
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:     "empty list",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("ListTenantKeys", mock.Anything, "t1").Return([]models.TenantKey{}, nil)
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name:     "with keys",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("ListTenantKeys", mock.Anything, "t1").Return([]models.TenantKey{
					{KeyID: "k1", TenantID: "t1", KeyPrefix: "rbtr_live_ab", CreatedAt: time.Now()},
				}, nil)
			},
			expectedCode: http.StatusOK,
			expectedLen:  1,
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
				http.MethodGet, nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			_ = deps.handleTenantKeysList(ctx)
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.expectedCode == http.StatusOK {
				var keys []models.TenantKey
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &keys))
				require.Len(t, keys, tc.expectedLen)
			}
		})
	}
}

func TestHandleTenantKeyRevoke(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("RevokeTenantKey", mock.Anything, "t1", "k1", mock.Anything).Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "not found",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("RevokeTenantKey", mock.Anything, "t1", "k1", mock.Anything).Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
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
				http.MethodPost, nil,
				testhelpers.Params{Names: []string{"tenant_id", "key_id"}, Values: []string{"t1", "k1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			_ = deps.handleTenantKeyRevoke(ctx)
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleTenantSetEnabled(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		body         string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:     "disable tenant",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     `{"enabled":false}`,
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("SetTenantEnabled", mock.Anything, "t1", false).Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "enable tenant",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     `{"enabled":true}`,
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("SetTenantEnabled", mock.Anything, "t1", true).Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "tenant not found",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     `{"enabled":false}`,
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("SetTenantEnabled", mock.Anything, "t1", false).Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
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

			body := tc.body
			if body == "" {
				body = `{}`
			}
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				testhelpers.MakeBody(json.RawMessage(body)),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			_ = deps.handleTenantSetEnabled(ctx)
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleTenantKeyRotate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		checkBody    bool
	}{
		{
			name:         "unauthorized",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:     "success with existing keys",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("ListTenantKeys", mock.Anything, "t1").Return([]models.TenantKey{
					{KeyID: "old_k1", TenantID: "t1", KeyPrefix: "rbtr_live_xx"},
				}, nil)
				m.On("RevokeTenantKey", mock.Anything, "t1", "old_k1", mock.Anything).Return(nil)
				m.On("CreateTenantKey", mock.Anything, mock.Anything).Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusOK,
			checkBody:    true,
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
				http.MethodPost, nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			_ = deps.handleTenantKeyRotate(ctx)
			require.Equal(t, tc.expectedCode, rec.Code)

			if tc.checkBody {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.NotEmpty(t, resp["api_key"])
				require.NotEmpty(t, resp["key_id"])
				require.NotEmpty(t, resp["key_prefix"])
			}
		})
	}
}
