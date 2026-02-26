package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type tenantStoreWithUpgrade struct {
	store.StoreAPI
	upgradeCalled bool
	upgradeOld    string
	upgradeNew    string
	upgradeErr    error
}

func (s *tenantStoreWithUpgrade) UpgradeTenantKeyHash(_ context.Context, oldKeyHash, newKeyHash string) error {
	s.upgradeCalled = true
	s.upgradeOld = oldKeyHash
	s.upgradeNew = newKeyHash
	return s.upgradeErr
}

func TestAuthenticateTenant(t *testing.T) {
	errStoreBoom := errors.New("boom")
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
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
			name:      "agent id too long",
			tenantKey: "key",
			agentID:   strings.Repeat("a", 129),
			err:       ErrInvalidAgentID,
		},
		{
			name:      "agent id invalid chars",
			tenantKey: "key",
			agentID:   "agent with spaces",
			err:       ErrInvalidAgentID,
		},
		{
			name:      "agent id valid chars",
			tenantKey: "key",
			agentID:   "agent_v2.0-beta",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", t.Context(), mock.Anything).
					Return(tenant, nil)
			},
			expected: tenant,
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

func TestAuthenticateTenantWithHashCandidates_LegacyUpgrade(t *testing.T) {
	rawKey := "tenant_demo_key"
	candidates := utils.BuildTenantKeyHashCandidates(rawKey, []string{"secret_current", "secret_previous"})
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	mockStore := store.NewMockStoreAPI(t)
	mockStore.On("GetTenantByKeyHash", t.Context(), candidates.Current).
		Return(models.Tenant{}, store.ErrNotFound).Once()
	mockStore.On("GetTenantByKeyHash", t.Context(), candidates.Previous[0]).
		Return(models.Tenant{}, store.ErrNotFound).Once()
	mockStore.On("GetTenantByKeyHash", t.Context(), candidates.Legacy).
		Return(tenant, nil).Once()

	upgradeStore := &tenantStoreWithUpgrade{StoreAPI: mockStore}

	result, err := authenticateTenantWithHashCandidates(t.Context(), upgradeStore, candidates)
	require.NoError(t, err)
	require.Equal(t, tenant, result.Tenant)
	require.True(t, result.LegacyHashMatched)
	require.True(t, result.LegacyHashUpgraded)
	require.True(t, upgradeStore.upgradeCalled)
	require.Equal(t, candidates.Legacy, upgradeStore.upgradeOld)
	require.Equal(t, candidates.Current, upgradeStore.upgradeNew)
}

func TestAuthenticateTenantWithHashCandidates_PreviousHMACMatchNoUpgrade(t *testing.T) {
	rawKey := "tenant_demo_key"
	candidates := utils.BuildTenantKeyHashCandidates(rawKey, []string{"secret_current", "secret_previous"})
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	mockStore := store.NewMockStoreAPI(t)
	mockStore.On("GetTenantByKeyHash", t.Context(), candidates.Current).
		Return(models.Tenant{}, store.ErrNotFound).Once()
	mockStore.On("GetTenantByKeyHash", t.Context(), candidates.Previous[0]).
		Return(tenant, nil).Once()

	upgradeStore := &tenantStoreWithUpgrade{StoreAPI: mockStore}

	result, err := authenticateTenantWithHashCandidates(t.Context(), upgradeStore, candidates)
	require.NoError(t, err)
	require.Equal(t, tenant, result.Tenant)
	require.False(t, result.LegacyHashMatched)
	require.False(t, result.LegacyHashUpgraded)
	require.False(t, upgradeStore.upgradeCalled)
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
		{
			name:  "granular read allowed by legacy read umbrella",
			key:   "key",
			scope: "admin:tools:read",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}, nil)
			},
			expected: models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}},
		},
		{
			name:  "granular write allowed by legacy write umbrella",
			key:   "key",
			scope: "admin:tools:write",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:write"}}, nil)
			},
			expected: models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:write"}},
		},
		{
			name:  "export allowed by legacy read umbrella",
			key:   "key",
			scope: "admin:audit:export",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}, nil)
			},
			expected: models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}},
		},
		{
			name:  "simulate allowed by legacy read umbrella",
			key:   "key",
			scope: "admin:policies:simulate",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}, nil)
			},
			expected: models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}},
		},
		{
			name:  "granular explicit scope works",
			key:   "key",
			scope: "admin:approvals:decide",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:approvals:decide"}}, nil)
			},
			expected: models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:approvals:decide"}},
		},
		{
			name:  "granular write denied for read-only umbrella",
			key:   "key",
			scope: "admin:settings:write",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}, nil)
			},
			err: ErrForbidden,
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

func TestAuthenticateAdminAny(t *testing.T) {
	adminKey := models.AdminKey{AdminKeyID: "admin", Scopes: []string{"admin:read"}}
	cases := []struct {
		name       string
		key        string
		storeSetup func(*store.MockStoreAPI)
		expected   models.AdminKey
		err        error
	}{
		{
			name: "missing admin key",
			err:  ErrUnauthorized,
		},
		{
			name: "key not found",
			key:  "key",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{}, store.ErrNotFound)
			},
			err: ErrUnauthorized,
		},
		{
			name: "store error",
			key:  "key",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminKeyByHash", t.Context(), mock.Anything).
					Return(models.AdminKey{}, errors.New("boom"))
			},
			err: errors.New("boom"),
		},
		{
			name: "success",
			key:  "key",
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
			got, err := AuthenticateAdminAny(t.Context(), storeAPI, tc.key)
			if tc.err != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err.Error())
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bearerToken(tc.value); got != tc.expect {
				t.Fatalf("expected %q got %q", tc.expect, got)
			}
		})
	}
}

func TestTenantKeyFromRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		authorization     string
		xTenantKey        string
		disableXTenantKey bool
		expectKey         string
		expectFallback    bool
	}{
		{
			name:           "bearer preferred over fallback",
			authorization:  "Bearer bearer_key",
			xTenantKey:     "legacy_key",
			expectKey:      "bearer_key",
			expectFallback: false,
		},
		{
			name:           "fallback used when bearer missing",
			xTenantKey:     "legacy_key",
			expectKey:      "legacy_key",
			expectFallback: true,
		},
		{
			name:              "fallback disabled",
			xTenantKey:        "legacy_key",
			disableXTenantKey: true,
			expectKey:         "",
			expectFallback:    false,
		},
		{
			name:           "no auth headers",
			expectKey:      "",
			expectFallback: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tc.authorization != "" {
				req.Header.Set(AuthorizationHeader, tc.authorization)
			}
			if tc.xTenantKey != "" {
				req.Header.Set(TenantKeyHeader, tc.xTenantKey)
			}

			key, fallback := TenantKeyFromRequest(req, tc.disableXTenantKey)
			if key != tc.expectKey {
				t.Fatalf("expected key %q got %q", tc.expectKey, key)
			}
			if fallback != tc.expectFallback {
				t.Fatalf("expected fallback %v got %v", tc.expectFallback, fallback)
			}
		})
	}
}

func TestHasScopeGranularCompatibilityMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		scopes   []string
		required string
		allowed  bool
	}{
		{name: "explicit tenants read", scopes: []string{"admin:tenants:read"}, required: "admin:tenants:read", allowed: true},
		{name: "legacy read allows tenants read", scopes: []string{"admin:read"}, required: "admin:tenants:read", allowed: true},
		{name: "legacy write does not imply read", scopes: []string{"admin:write"}, required: "admin:tenants:read", allowed: false},
		{name: "explicit tenants write", scopes: []string{"admin:tenants:write"}, required: "admin:tenants:write", allowed: true},
		{name: "legacy write allows tenants write", scopes: []string{"admin:write"}, required: "admin:tenants:write", allowed: true},
		{name: "legacy read does not allow tenants write", scopes: []string{"admin:read"}, required: "admin:tenants:write", allowed: false},
		{name: "legacy write allows keys rotate", scopes: []string{"admin:write"}, required: "admin:keys:rotate", allowed: true},
		{name: "legacy write allows policy publish", scopes: []string{"admin:write"}, required: "admin:policies:publish", allowed: true},
		{name: "legacy read allows policy simulate", scopes: []string{"admin:read"}, required: "admin:policies:simulate", allowed: true},
		{name: "legacy read allows audit export", scopes: []string{"admin:read"}, required: "admin:audit:export", allowed: true},
		{name: "legacy write allows notification test", scopes: []string{"admin:write"}, required: "admin:notifications:test", allowed: true},
		{name: "legacy write allows settings write", scopes: []string{"admin:write"}, required: "admin:settings:write", allowed: true},
		{name: "explicit mismatch denied", scopes: []string{"admin:tools:read"}, required: "admin:tools:write", allowed: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.allowed, hasScope(tc.scopes, tc.required))
		})
	}
}
