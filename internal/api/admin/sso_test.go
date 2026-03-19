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
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleSSOConfigGet(t *testing.T) {
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
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:settings:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetSSOConfig", context.Background()).
					Return(store.SSOConfig{
						Enabled:         true,
						Issuer:          "https://idp.example.com",
						ClientID:        "client-123",
						ClientSecretRef: "vault:secret/sso",
						RedirectURI:     "http://localhost/callback",
						AllowedDomains:  "example.com",
						DefaultScopes:   "admin:read,admin:write",
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

			ctx, req, rec := testhelpers.MakeRequest(http.MethodGet, nil, nil)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleSSOConfigGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				var resp SSOConfigResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.True(t, resp.Enabled)
				require.Equal(t, "https://idp.example.com", resp.Issuer)
				require.Equal(t, "client-123", resp.ClientID)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleSSOEnabledUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      BooleanSettingRequest
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
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  BooleanSettingRequest{Enabled: true},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetSSOEnabled", context.Background()).Return(false, nil)
				storeMock.On("SetSSOEnabled", context.Background(), true).Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
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

			ctx, req, rec := testhelpers.MakeRequest(http.MethodPut, nil, testhelpers.MakeBody(tc.payload))
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleSSOEnabledUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleSSOConfigUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      SSOConfigRequest
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
			name:     "missing issuer",
			adminKey: "key",
			scopes:   []string{"admin:settings:write"},
			payload: SSOConfigRequest{
				ClientID: "client-123",
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "missing client_id",
			adminKey: "key",
			scopes:   []string{"admin:settings:write"},
			payload: SSOConfigRequest{
				Issuer: "https://idp.example.com",
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:settings:write"},
			payload: SSOConfigRequest{
				Issuer:          "https://idp.example.com",
				ClientID:        "client-123",
				ClientSecretRef: "vault:secret/sso",
				RedirectURI:     "http://localhost/callback",
				AllowedDomains:  "example.com",
				DefaultScopes:   "admin:read",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetSSOConfig", context.Background()).
					Return(store.SSOConfig{Issuer: "old-issuer", ClientID: "old-client"}, nil)
				storeMock.On("SetSSOConfig", context.Background(),
					"https://idp.example.com", "client-123", "vault:secret/sso",
					"http://localhost/callback", "example.com", "admin:read",
				).Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
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

			ctx, req, rec := testhelpers.MakeRequest(http.MethodPut, nil, testhelpers.MakeBody(tc.payload))
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleSSOConfigUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleSSOAuthorizeNoProvider(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)

	ctx, _, rec := testhelpers.MakeRequest(http.MethodGet, nil, nil)

	deps := Dependencies{
		Store:   storeMock,
		Metrics: newTestMetrics(),
		Config:  config.Config{},
		// OIDCProvider and AdminSessionMgr are nil
	}
	err := deps.handleSSOAuthorize(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotImplemented, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "SSO not configured", resp["error"])
}

func TestHandleSSOLogout(t *testing.T) {
	t.Run("no admin session manager", func(t *testing.T) {
		storeMock := store.NewMockStoreAPI(t)
		ctx, _, rec := testhelpers.MakeRequest(http.MethodPost, nil, nil)

		deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
		err := deps.handleSSOLogout(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotImplemented, rec.Code)
	})

	t.Run("revokes valid session", func(t *testing.T) {
		storeMock := store.NewMockStoreAPI(t)

		mgr, err := auth.NewAdminSessionManager(15 * time.Minute)
		require.NoError(t, err)

		user := auth.OIDCUserInfo{Email: "alice@example.com", Name: "Alice", Subject: "sub_123"}
		token, _, err := mgr.IssueSession(user, []string{"admin:read"})
		require.NoError(t, err)

		ctx, req, rec := testhelpers.MakeRequest(http.MethodPost, nil, nil)
		req.Header.Set(auth.AuthorizationHeader, "Bearer "+token)

		deps := Dependencies{
			Store:           storeMock,
			Metrics:         newTestMetrics(),
			Config:          config.Config{},
			AdminSessionMgr: mgr,
		}
		err = deps.handleSSOLogout(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, rec.Code)

		// Verify the session is actually revoked.
		_, err = mgr.ValidateSession(token)
		require.Error(t, err)
	})

	t.Run("not an SSO session", func(t *testing.T) {
		storeMock := store.NewMockStoreAPI(t)

		mgr, err := auth.NewAdminSessionManager(15 * time.Minute)
		require.NoError(t, err)

		ctx, req, rec := testhelpers.MakeRequest(http.MethodPost, nil, nil)
		req.Header.Set(auth.AuthorizationHeader, "Bearer plainapikey")

		deps := Dependencies{
			Store:           storeMock,
			Metrics:         newTestMetrics(),
			Config:          config.Config{},
			AdminSessionMgr: mgr,
		}
		err = deps.handleSSOLogout(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestParseCSV(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty string", input: "", expected: nil},
		{name: "whitespace only", input: "   ", expected: nil},
		{name: "single value", input: "admin:read", expected: []string{"admin:read"}},
		{name: "multiple values", input: "admin:read,admin:write", expected: []string{"admin:read", "admin:write"}},
		{name: "values with spaces", input: " admin:read , admin:write ", expected: []string{"admin:read", "admin:write"}},
		{name: "trailing comma", input: "a,b,", expected: []string{"a", "b"}},
		{name: "leading comma", input: ",a,b", expected: []string{"a", "b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, parseCSV(tc.input))
		})
	}
}
