package auth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

func TestAuthenticateTenant(t *testing.T) {
	errStoreBoom := errors.New("boom")
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant"}
	cases := []struct {
		name       string
		tenantKey  string
		agentID    string
		storeSetup func(*store.MockStoreAPI)
		expected   models.Tenant
		err        error
	}{
		{
			name:    "missing tenant key",
			agentID: "agent",
			err:     ErrUnauthorized,
		},
		{
			name:      "missing agent id",
			tenantKey: "key",
			err:       ErrForbidden,
		},
		{
			name:      "tenant key not found",
			tenantKey: "key",
			agentID:   "agent",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", t.Context(), mock.Anything).
					Return(models.Tenant{}, store.ErrNotFound)
			},
			err: ErrUnauthorized,
		},
		{
			name:      "store error",
			tenantKey: "key",
			agentID:   "agent",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", t.Context(), mock.Anything).
					Return(models.Tenant{}, errStoreBoom)
			},
			err: errStoreBoom,
		},
		{
			name:      "success",
			tenantKey: "key",
			agentID:   "agent",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", t.Context(), mock.Anything).
					Return(tenant, nil)
			},
			expected: tenant,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStoreAPI(t)
			var storeAPI store.StoreAPI = mockStore
			if tc.storeSetup != nil {
				tc.storeSetup(mockStore)
			}
			got, err := AuthenticateTenant(t.Context(), storeAPI, tc.tenantKey, tc.agentID)
			if tc.err != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, tc.err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestAuthenticateAdmin(t *testing.T) {
	adminKey := models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:write"}}
	cases := []struct {
		name       string
		key        string
		scope      string
		storeSetup func(*store.MockStoreAPI)
		expected   models.AdminKey
		err        error
	}{
		{
			name:  "missing admin key",
			scope: "admin:write",
			err:   ErrUnauthorized,
		},
		{
			name:  "key not found",
			key:   "key",
			scope: "admin:write",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{}, store.ErrNotFound)
			},
			err: ErrUnauthorized,
		},
		{
			name:  "missing scope",
			key:   "key",
			scope: "admin:write",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}, nil)
			},
			err: ErrForbidden,
		},
		{
			name:  "success",
			key:   "key",
			scope: "admin:write",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(adminKey, nil)
			},
			expected: adminKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := store.NewMockStoreAPI(t)
			var storeAPI store.StoreAPI = mockStore
			if tc.storeSetup != nil {
				tc.storeSetup(mockStore)
			}
			got, err := AuthenticateAdmin(t.Context(), storeAPI, tc.key, tc.scope)
			if tc.err != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, tc.err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestAdminKeyFromRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		headerKey string
		headerVal string
		expect    string
	}{
		{
			name:      "bearer token",
			headerKey: AuthorizationHeader,
			headerVal: "Bearer token123",
			expect:    "token123",
		},
		{
			name:      "x-admin-key fallback",
			headerKey: AdminKeyHeader,
			headerVal: "adminkey",
			expect:    "adminkey",
		},
		{
			name:      "invalid bearer format",
			headerKey: AuthorizationHeader,
			headerVal: "Bearer",
			expect:    "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(tc.headerKey, tc.headerVal)
			if got := AdminKeyFromRequest(req); got != tc.expect {
				t.Fatalf("expected %q got %q", tc.expect, got)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		value  string
		expect string
	}{
		{name: "empty", value: "", expect: ""},
		{name: "no space", value: "Bearer", expect: ""},
		{name: "wrong scheme", value: "Token abc", expect: ""},
		{name: "valid", value: "Bearer abc", expect: "abc"},
		{name: "trimmed", value: "Bearer   abc ", expect: "abc"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bearerToken(tc.value); got != tc.expect {
				t.Fatalf("expected %q got %q", tc.expect, got)
			}
		})
	}
}
