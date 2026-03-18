package admin

import (
	"bytes"
	"context"
	"encoding/csv"
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
		name                    string
		adminKey                string
		scopes                  []string
		tenantID                string
		depsConfig              config.Config
		storeSetup              func(*store.MockStoreAPI)
		expectedCode            int
		expectedErr             bool
		expectMode              string
		expectPassThroughToolID string
		expectDisableXTenantKey bool
		expectRateLimiting      bool
		expectArgConstraints    bool
		expectRateLimitPerMin   int64
		expectRateLimitPerDay   int64
		expectRateLimitScope    string
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
				storeMock.On("GetAuditRetentionDays", context.Background()).Return(365, nil)
				storeMock.On("GetDefaultRateLimitConfig", context.Background()).Return(models.RateLimitConfig{
					PerMinute: 60,
					PerDay:    10000,
					Scope:     "tenant_agent_tool",
				}, nil)
				storeMock.On("GetDisableXTenantKey", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureRateLimiting", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureArgConstraints", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureSessionTokens", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureFileGovernance", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSessionTokenTTLSeconds", context.Background()).Return(0, store.ErrNotFound)
				storeMock.On("GetSecretProviderAWS", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderGCP", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderVault", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderAzure", context.Background()).Return(false, store.ErrNotFound)
			},
			expectedCode:          http.StatusOK,
			expectMode:            "enforce",
			expectRateLimitPerMin: 60,
			expectRateLimitPerDay: 10000,
			expectRateLimitScope:  "tenant_agent_tool",
		},
		{
			name:     "success with runtime feature flags",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			depsConfig: config.Config{
				DisableXTenantKey:     true,
				FeatureRateLimiting:   true,
				FeatureArgConstraints: true,
				FeatureShadowMode:     true,
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminWriteLock", context.Background()).Return(false, nil)
				storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(900, nil)
				storeMock.On("GetAuditRetentionDays", context.Background()).Return(365, nil)
				storeMock.On("GetDefaultRateLimitConfig", context.Background()).Return(models.RateLimitConfig{
					PerMinute: 120,
					PerDay:    15000,
					Scope:     "tenant_tool",
				}, nil)
				storeMock.On("GetDisableXTenantKey", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureRateLimiting", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureArgConstraints", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureSessionTokens", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureFileGovernance", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSessionTokenTTLSeconds", context.Background()).Return(0, store.ErrNotFound)
				storeMock.On("GetSecretProviderAWS", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderGCP", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderVault", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderAzure", context.Background()).Return(false, store.ErrNotFound)
			},
			expectedCode:            http.StatusOK,
			expectMode:              "enforce",
			expectDisableXTenantKey: true,
			expectRateLimiting:      true,
			expectArgConstraints:    true,
			expectRateLimitPerMin:   120,
			expectRateLimitPerDay:   15000,
			expectRateLimitScope:    "tenant_tool",
		},
		{
			name:     "success with tenant settings",
			adminKey: "key",
			scopes:   []string{"admin:read"},
			tenantID: "t1",
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAdminWriteLock", context.Background()).Return(false, nil)
				storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(900, nil)
				storeMock.On("GetAuditRetentionDays", context.Background()).Return(365, nil)
				storeMock.On("GetDefaultRateLimitConfig", context.Background()).Return(models.RateLimitConfig{
					PerMinute: 75,
					PerDay:    12000,
					Scope:     "tenant_agent",
				}, nil)
				storeMock.On("GetDisableXTenantKey", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureRateLimiting", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureArgConstraints", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureSessionTokens", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetFeatureFileGovernance", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSessionTokenTTLSeconds", context.Background()).Return(0, store.ErrNotFound)
				storeMock.On("GetSecretProviderAWS", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderGCP", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderVault", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetSecretProviderAzure", context.Background()).Return(false, store.ErrNotFound)
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID:                     "t1",
					ActivePolicyVersion:          "p1",
					EnforcementMode:              "shadow",
					MCPPassthroughUpstreamToolID: "mcp_upstream",
				}, nil)
			},
			expectedCode:            http.StatusOK,
			expectMode:              "shadow",
			expectPassThroughToolID: "mcp_upstream",
			expectRateLimitPerMin:   75,
			expectRateLimitPerDay:   12000,
			expectRateLimitScope:    "tenant_agent",
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
			if tc.tenantID != "" {
				req.URL.RawQuery = "tenant_id=" + tc.tenantID
			}
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: tc.depsConfig}
			err := deps.handleSettingsGet(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
			if tc.expectedCode == http.StatusOK && tc.expectMode != "" {
				var response SettingsResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
				require.Equal(t, tc.expectMode, response.EnforcementMode)
				require.Equal(t, tc.expectPassThroughToolID, response.MCPPassthroughUpstreamToolID)
				require.Equal(t, tc.expectDisableXTenantKey, response.DisableXTenantKey)
				require.Equal(t, tc.expectRateLimiting, response.FeatureRateLimiting)
				require.Equal(t, tc.expectArgConstraints, response.FeatureArgConstraints)
				require.Equal(t, tc.expectRateLimitPerMin, response.DefaultRateLimitPerMinute)
				require.Equal(t, tc.expectRateLimitPerDay, response.DefaultRateLimitPerDay)
				require.Equal(t, tc.expectRateLimitScope, response.DefaultRateLimitScope)
			}
		})
	}
}

func TestHandleEnforcementModeUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      EnforcementModeRequest
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
			name:         "invalid payload",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      EnforcementModeRequest{TenantID: "t1", EnforcementMode: "invalid"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload: EnforcementModeRequest{
				TenantID:        "t1",
				EnforcementMode: "shadow",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID:            "t1",
					ActivePolicyVersion: "p1",
					EnforcementMode:     "enforce",
				}, nil)
				storeMock.On("SetTenantEnforcementMode", context.Background(), "t1", "shadow").Return(nil)
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
			err := deps.handleEnforcementModeUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleMCPPassthroughUpstreamUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      MCPPassthroughUpstreamRequest
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
			name:         "invalid payload missing tenant",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      MCPPassthroughUpstreamRequest{ToolID: "mcp_a"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "tool not found",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload: MCPPassthroughUpstreamRequest{
				TenantID: "t1",
				ToolID:   "missing",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID: "t1",
				}, nil)
				storeMock.On("GetTool", context.Background(), "t1", "missing").Return(models.Tool{}, store.ErrNotFound)
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:     "invalid tool transport",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload: MCPPassthroughUpstreamRequest{
				TenantID: "t1",
				ToolID:   "rest_tool",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID: "t1",
				}, nil)
				storeMock.On("GetTool", context.Background(), "t1", "rest_tool").Return(models.Tool{
					ToolID:    "rest_tool",
					TenantID:  "t1",
					Transport: "http_api",
				}, nil)
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success set upstream",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload: MCPPassthroughUpstreamRequest{
				TenantID: "t1",
				ToolID:   "mcp_tool",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID: "t1",
				}, nil)
				storeMock.On("GetTool", context.Background(), "t1", "mcp_tool").Return(models.Tool{
					ToolID:         "mcp_tool",
					TenantID:       "t1",
					Transport:      "mcp_streamable_http",
					MCPUpstreamURL: "http://upstream.example",
				}, nil)
				storeMock.On("SetTenantMCPPassthroughUpstreamToolID", context.Background(), "t1", "mcp_tool").Return(nil)
				storeMock.On("InsertAuditEvent", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name:     "success clear upstream",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload: MCPPassthroughUpstreamRequest{
				TenantID: "t1",
				ToolID:   "",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
					TenantID:                     "t1",
					MCPPassthroughUpstreamToolID: "mcp_tool",
				}, nil)
				storeMock.On("SetTenantMCPPassthroughUpstreamToolID", context.Background(), "t1", "").Return(nil)
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
			err := deps.handleMCPPassthroughUpstreamUpdate(ctx)
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

func TestHandleDefaultRateLimitUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      DefaultRateLimitRequest
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
			payload:      DefaultRateLimitRequest{PerMinute: 60, PerDay: 10000, Scope: "tenant_agent_tool"},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:         "invalid payload",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      DefaultRateLimitRequest{PerMinute: 0, PerDay: 10000, Scope: "tenant_agent_tool"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "invalid scope",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      DefaultRateLimitRequest{PerMinute: 60, PerDay: 10000, Scope: "invalid"},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  DefaultRateLimitRequest{PerMinute: 120, PerDay: 20000, Scope: "tenant"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetDefaultRateLimitConfig", context.Background()).Return(models.RateLimitConfig{
					PerMinute: 60,
					PerDay:    10000,
					Scope:     "tenant_agent_tool",
				}, nil)
				storeMock.On("SetDefaultRateLimitConfig", context.Background(), int64(120), int64(20000), "tenant").Return(nil)
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
			err := deps.handleDefaultRateLimitUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleAuditRetentionUpdate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		payload      AuditRetentionRequest
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
			payload:      AuditRetentionRequest{Days: 365},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:         "invalid payload",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			payload:      AuditRetentionRequest{Days: 7},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  AuditRetentionRequest{Days: 365},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetAuditRetentionDays", context.Background()).Return(180, nil)
				storeMock.On("SetAuditRetentionDays", context.Background(), 365).Return(nil)
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
			err := deps.handleAuditRetentionUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleDisableXTenantKeyUpdate(t *testing.T) {
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
			name:         "forbidden",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			payload:      BooleanSettingRequest{Enabled: true},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  BooleanSettingRequest{Enabled: true},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetDisableXTenantKey", context.Background()).Return(false, nil)
				storeMock.On("SetDisableXTenantKey", context.Background(), true).Return(nil)
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
			err := deps.handleDisableXTenantKeyUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleFeatureRateLimitingUpdate(t *testing.T) {
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
			name:         "forbidden",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			payload:      BooleanSettingRequest{Enabled: true},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  BooleanSettingRequest{Enabled: true},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetFeatureRateLimiting", context.Background()).Return(false, nil)
				storeMock.On("SetFeatureRateLimiting", context.Background(), true).Return(nil)
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
			err := deps.handleFeatureRateLimitingUpdate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandleFeatureArgConstraintsUpdate(t *testing.T) {
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
			name:         "forbidden",
			adminKey:     "key",
			scopes:       []string{"admin:read"},
			payload:      BooleanSettingRequest{Enabled: true},
			expectedCode: http.StatusForbidden,
			expectedErr:  true,
		},
		{
			name:     "success",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			payload:  BooleanSettingRequest{Enabled: true},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetFeatureArgConstraints", context.Background()).Return(false, nil)
				storeMock.On("SetFeatureArgConstraints", context.Background(), true).Return(nil)
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
			err := deps.handleFeatureArgConstraintsUpdate(ctx)
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
				storeMock.On("ListAuditEvents", context.Background(), "t1", 50, 0, "", "", "", mock.Anything, mock.Anything).
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

func TestWriteAuditCSV(t *testing.T) {
	events := []models.AdminAuditEvent{
		{
			AuditEventID: "ae_1",
			TenantID:     "t1",
			StreamID:     "t1",
			EventHash:    "hash",
			PrevHash:     "prev",
			ActorType:    "admin_key",
			ActorID:      "admin",
			Action:       "ACTION",
			ResourceType: "RESOURCE",
			ResourceID:   "res",
			RequestID:    "req",
			IP:           "127.0.0.1",
			UserAgent:    "agent",
			Before:       []byte(`{"ok":true}`),
			After:        []byte(`{"ok":false}`),
			CreatedAt:    time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC),
		},
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	err := writeAuditCSV(writer, events, true)
	writer.Flush()
	require.NoError(t, err)
	reader := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(records), 2)
	require.Equal(t, "audit_event_id", records[0][0])
	require.Equal(t, "ae_1", records[1][0])
	require.Contains(t, records[1][len(records[1])-2], "\"ok\":true")
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
				storeMock.On("ListAuditEvents", context.Background(), "", 50, 0, "", "", "", mock.Anything, mock.Anything).
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
