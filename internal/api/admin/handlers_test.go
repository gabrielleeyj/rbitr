package admin

import (
	"context"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func TestHandleTenantConfigUpdate(t *testing.T) {
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
			name:         "forbidden",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenant", context.Background(), "t1").
					Return(models.TenantSummary{TenantID: "t1", Name: "Tenant"}, nil)
				storeMock.On("GetTenantKeyHash", context.Background(), "t1").Return("hash", nil)
				storeMock.On("UpdateTenantConfig", context.Background(), "t1", "Tenant", "newkey").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "write lock",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenant", context.Background(), "t1").
					Return(models.TenantSummary{TenantID: "t1", Name: "Tenant"}, nil)
				storeMock.On("GetTenantKeyHash", context.Background(), "t1").Return("hash", nil)
				storeMock.On("UpdateTenantConfig", context.Background(), "t1", "Tenant", "newkey").Return(store.ErrAdminWriteLocked)
			},
			expectedCode: http.StatusForbidden,
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

			payload := TenantConfigRequest{Name: "Tenant", TenantKey: "newkey"}
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				testhelpers.MakeBody(payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleTenantConfigUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleToolConfigUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      ToolConfigRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      ToolConfigRequest{BaseURL: "http://example"},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "missing base url",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      ToolConfigRequest{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  ToolConfigRequest{BaseURL: "http://example", AuthType: "bearer", AuthValue: "token"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTool", context.Background(), "t1", "tool").
					Return(models.Tool{ToolID: "tool", TenantID: "t1", BaseURL: "old", AuthType: "bearer", AuthValue: "old"}, nil)
				storeMock.On("UpdateToolConfig", context.Background(), "t1", "tool", "http://example", "bearer", "token").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "write lock",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  ToolConfigRequest{BaseURL: "http://example", AuthType: "bearer", AuthValue: "token"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTool", context.Background(), "t1", "tool").
					Return(models.Tool{ToolID: "tool", TenantID: "t1", BaseURL: "old", AuthType: "bearer", AuthValue: "old"}, nil)
				storeMock.On("UpdateToolConfig", context.Background(), "t1", "tool", "http://example", "bearer", "token").Return(store.ErrAdminWriteLocked)
			},
			expectedCode: http.StatusForbidden,
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
				http.MethodPut,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id", "tool_id"}, Values: []string{"t1", "tool"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleToolConfigUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      PolicyUpdateRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      PolicyUpdateRequest{RegoModule: "module", PolicyVersion: "p_v2"},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "missing fields",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      PolicyUpdateRequest{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  PolicyUpdateRequest{RegoModule: "module", PolicyVersion: "p_v2"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_old"}, nil).Once()
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_v2"}, nil).Once()
				storeMock.On("UpdatePolicy", context.Background(), "t1", "module", "p_v2").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "write lock",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  PolicyUpdateRequest{RegoModule: "module", PolicyVersion: "p_v2"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_old"}, nil)
				storeMock.On("UpdatePolicy", context.Background(), "t1", "module", "p_v2").Return(store.ErrAdminWriteLocked)
			},
			expectedCode: http.StatusForbidden,
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
				http.MethodPut,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleBootstrapComplete(t *testing.T) {
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
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetBootstrapComplete", context.Background()).Return(false, nil)
				storeMock.On("MarkBootstrapComplete", context.Background()).Return(nil)
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

			ctx, req, rec := testhelpers.MakeRequest(http.MethodPut, nil, nil)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleBootstrapComplete(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleAdminWriteLock(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      AdminWriteLockRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      AdminWriteLockRequest{Locked: true},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  AdminWriteLockRequest{Locked: true},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminWriteLock", context.Background()).Return(false, nil)
				storeMock.On("SetAdminWriteLock", context.Background(), true).Return(nil)
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
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleAdminWriteLock(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleRiskOverrideUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      RiskOverrideRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      RiskOverrideRequest{ActionRisk: "HIGH"},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "invalid risk",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      RiskOverrideRequest{ActionRisk: "EXTREME"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  RiskOverrideRequest{ActionRisk: "HIGH"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetRiskOverride", context.Background(), "t1", "DATA.EXPORT").Return("LOW", nil)
				storeMock.On("UpdateRiskOverride", context.Background(), "t1", "DATA.EXPORT", "HIGH").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "write lock",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  RiskOverrideRequest{ActionRisk: "HIGH"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetRiskOverride", context.Background(), "t1", "DATA.EXPORT").Return("LOW", nil)
				storeMock.On("UpdateRiskOverride", context.Background(), "t1", "DATA.EXPORT", "HIGH").Return(store.ErrAdminWriteLocked)
			},
			expectedCode: http.StatusForbidden,
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
				http.MethodPut,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id", "action_type"}, Values: []string{"t1", "DATA.EXPORT"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AdminKeyHeader, tc.adminKey)
			}

			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{Store: storeAPI, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleRiskOverrideUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func newTestMetrics() *telemetry.Metrics {
	return &telemetry.Metrics{
		DecisionsTotal:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_decisions_total_admin"}, []string{"decision", "action_type"}),
		GatewayRequests:        prometheus.NewCounter(prometheus.CounterOpts{Name: "test_gateway_requests_total_admin"}),
		ToolExecTotal:          prometheus.NewCounter(prometheus.CounterOpts{Name: "test_tool_exec_total_admin"}),
		ErrorsTotal:            prometheus.NewCounter(prometheus.CounterOpts{Name: "test_errors_total_admin"}),
		DecisionLatencyMs:      prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_decision_latency_ms_admin"}),
		ToolLatencyMs:          prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_tool_latency_ms_admin"}),
		PolicyEvalInvalidTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_policy_eval_invalid_total_admin"}, []string{"reason"}),
		ApprovalsCreatedTotal:  prometheus.NewCounter(prometheus.CounterOpts{Name: "test_approvals_created_total_admin"}),
		ApprovalsResolvedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_approvals_resolved_total_admin"}, []string{"resolution"}),
		ApprovalsExecuteTotal:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_approvals_execute_total_admin"}, []string{"result"}),
	}
}

func modelsAdminKey(scopes []string) models.AdminKey {
	return models.AdminKey{AdminKeyID: "admin", Scopes: scopes}
}
