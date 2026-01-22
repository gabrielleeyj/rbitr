package admin

import (
	"context"
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

const sampleRegoModule = `package rbitr.policy
import rego.v1

decision := {
  "version": "2026-01-20",
  "decision": "ALLOW",
  "risk": "LOW",
  "rule": {"id": "rule_allow", "priority": 10},
  "reasons": [{"code": "ALLOW", "message": "ok"}],
  "constraints": {},
  "tags": []
}
`

func TestHandleTenantList(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListTenants", context.Background()).
					Return([]models.TenantSummary{{TenantID: "t1", Name: "Tenant"}}, nil)
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
			err := deps.handleTenantList(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleTenantDetail(t *testing.T) {
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
				storeMock.On("GetTenant", context.Background(), "t1").
					Return(models.TenantSummary{}, store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenant", context.Background(), "t1").
					Return(models.TenantSummary{TenantID: "t1", Name: "Tenant"}, nil)
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
			err := deps.handleTenantDetail(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleEvidenceList(t *testing.T) {
	cases := []struct {
		name         string
		query        string
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
			name:         "invalid limit",
			query:        "limit=bad",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			query:    "limit=10",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListEvidenceFiltered", context.Background(), "t1", "", "", "", (*time.Time)(nil), 10).
					Return([]models.ActionDecisionRecord{}, nil)
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
			req.URL.RawQuery = tc.query
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleEvidenceList(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyVersions(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListPolicyVersions", context.Background(), "t1").
					Return([]models.PolicyVersion{{TenantID: "t1", PolicyVersion: "p_v1"}}, nil)
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_v1"}, nil)
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
			err := deps.handlePolicyVersions(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyVersionGet(t *testing.T) {
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
				storeMock.On("GetPolicyVersion", context.Background(), "t1", "p_v1").
					Return(models.PolicyVersion{}, store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetPolicyVersion", context.Background(), "t1", "p_v1").
					Return(models.PolicyVersion{TenantID: "t1", PolicyVersion: "p_v1", RegoModule: "module"}, nil)
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
				testhelpers.Params{Names: []string{"tenant_id", "policy_version"}, Values: []string{"t1", "p_v1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyVersionGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyCreate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      PolicyCreateRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      PolicyCreateRequest{PolicyVersion: "p_v2", RegoModule: sampleRegoModule},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "invalid payload",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      PolicyCreateRequest{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  PolicyCreateRequest{PolicyVersion: "p_v2", RegoModule: sampleRegoModule, Notes: "notes"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("CreatePolicyVersion", context.Background(), "t1", "p_v2", sampleRegoModule, "admin", "notes").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
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
				http.MethodPost,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyCreate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyPublish(t *testing.T) {
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
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_old"}, nil)
				storeMock.On("PublishPolicyVersion", context.Background(), "t1", "p_v2").
					Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_old"}, nil).Once()
				storeMock.On("PublishPolicyVersion", context.Background(), "t1", "p_v2").
					Return(nil)
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_v2"}, nil).Once()
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

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				nil,
				testhelpers.Params{Names: []string{"tenant_id", "policy_version"}, Values: []string{"t1", "p_v2"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyPublish(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyRollback(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      PolicyRollbackRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      PolicyRollbackRequest{PolicyVersion: "p_v1"},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "not found",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  PolicyRollbackRequest{PolicyVersion: "p_v1"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_old"}, nil)
				storeMock.On("RollbackPolicyVersion", context.Background(), "t1", "p_v1").
					Return(store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  PolicyRollbackRequest{PolicyVersion: "p_v1"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_v2"}, nil).Once()
				storeMock.On("RollbackPolicyVersion", context.Background(), "t1", "p_v1").
					Return(nil)
				storeMock.On("GetTenantConfig", context.Background(), "t1").
					Return(models.TenantConfig{TenantID: "t1", ActivePolicyVersion: "p_v1"}, nil).Once()
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

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPut,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyRollback(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicySimulate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      PolicySimulationRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			payload:      PolicySimulationRequest{Input: map[string]any{"tenant_id": "t1"}},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:         "missing input",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			payload:      PolicySimulationRequest{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "success",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			payload:      PolicySimulationRequest{RegoModule: sampleRegoModule, Input: map[string]any{"tenant_id": "t1"}},
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
				http.MethodPost,
				testhelpers.MakeBody(tc.payload),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicySimulate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleRiskOverridesList(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListRiskOverrides", context.Background(), "t1").
					Return([]models.RiskOverride{}, nil)
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
			err := deps.handleRiskOverridesList(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleRiskOverrideDelete(t *testing.T) {
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
				storeMock.On("GetRiskOverride", context.Background(), "t1", "DATA.EXPORT").
					Return("HIGH", nil)
				storeMock.On("DeleteRiskOverride", context.Background(), "t1", "DATA.EXPORT").Return(nil)
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

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodDelete,
				nil,
				testhelpers.Params{Names: []string{"tenant_id", "action_type"}, Values: []string{"t1", "DATA.EXPORT"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handleRiskOverrideDelete(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleToolsList(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListTools", context.Background(), "t1").
					Return([]models.Tool{{ToolID: "tool", TenantID: "t1"}}, nil)
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
			err := deps.handleToolsList(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleSettingsGet(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminWriteLock", context.Background()).Return(false, nil)
				storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(900, nil)
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
			err := deps.handleSettingsGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleDefaultApprovalTTLUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      DefaultApprovalTTLRequest
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
			payload:      DefaultApprovalTTLRequest{Seconds: 900},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:         "invalid payload",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      DefaultApprovalTTLRequest{Seconds: 30},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  DefaultApprovalTTLRequest{Seconds: 900},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(600, nil)
				storeMock.On("SetDefaultApprovalTTLSeconds", context.Background(), 900).Return(nil)
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
			err := deps.handleDefaultApprovalTTLUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleAuditList(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListAuditEvents", context.Background(), "t1", 50, 0, "", "", "").
					Return([]models.AdminAuditEvent{}, nil)
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
			err := deps.handleAuditList(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleAuditListAll(t *testing.T) {
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
			scopes:   []string{"admin:read"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("ListAuditEvents", context.Background(), "", 50, 0, "", "", "").
					Return([]models.AdminAuditEvent{}, nil)
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
			err := deps.handleAuditListAll(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}
