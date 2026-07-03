package admin

import (
	"bytes"
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
	"github.com/gabrielleeyj/rbitr/internal/policy/compiler"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

func structuredBody(t *testing.T, req *PolicyStructuredCreateRequest) *bytes.Buffer {
	t.Helper()
	data, err := json.Marshal(req)
	require.NoError(t, err)
	return bytes.NewBuffer(data)
}

func validStructured() compiler.StructuredPolicy {
	return compiler.StructuredPolicy{
		SchemaVersion: "1",
		DefaultEffect: compiler.EffectDeny,
		Rules: []compiler.Rule{
			{ID: "allow_reads", Priority: 100, Effect: compiler.EffectAllow, Match: compiler.Matcher{ActionTypes: []string{"DATA.READ"}}},
		},
	}
}

func TestHandlePolicyStructuredCreate(t *testing.T) {
	cases := []struct {
		name         string
		adminKey     string
		scopes       []string
		body         PolicyStructuredCreateRequest
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
		expectedErr  bool
	}{
		{
			name:         "unauthorized",
			body:         PolicyStructuredCreateRequest{PolicyVersion: "p1", Structured: validStructured()},
			expectedCode: http.StatusUnauthorized,
			expectedErr:  true,
		},
		{
			name:     "compile failure returns 400",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body: PolicyStructuredCreateRequest{
				PolicyVersion: "p1",
				Structured:    compiler.StructuredPolicy{DefaultEffect: "BOGUS"},
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "missing version returns 400",
			adminKey:     "key",
			scopes:       []string{"admin:write"},
			body:         PolicyStructuredCreateRequest{Structured: validStructured()},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "success without publish",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     PolicyStructuredCreateRequest{PolicyVersion: "p1", Structured: validStructured()},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("CreatePolicyVersionStructured", mock.Anything, "t1", "p1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:     "success with publish",
			adminKey: "key",
			scopes:   []string{"admin:write"},
			body:     PolicyStructuredCreateRequest{PolicyVersion: "p1", Structured: validStructured(), Publish: true},
			storeSetup: func(m *store.MockStoreAPI) {
				m.On("CreatePolicyVersionStructured", mock.Anything, "t1", "p1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
				m.On("GetTenantConfig", mock.Anything, "t1").Return(models.TenantConfig{ActivePolicyVersion: "p0"}, nil)
				m.On("PublishPolicyVersion", mock.Anything, "t1", "p1").Return(nil)
				m.On("InsertAuditEvent", mock.Anything, mock.Anything).Return(nil)
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

			body := tc.body
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				structuredBody(t, &body),
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
			)
			if tc.adminKey != "" {
				req.Header.Set(auth.AuthorizationHeader, "Bearer "+tc.adminKey)
			}

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			err := deps.handlePolicyStructuredCreate(ctx)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedCode, rec.Code)
		})
	}
}

func TestHandlePolicyStructuredGet(t *testing.T) {
	structured := validStructured()
	structuredJSON, err := json.Marshal(structured)
	require.NoError(t, err)

	cases := []struct {
		name             string
		version          models.PolicyVersion
		wantAdvancedMode bool
	}{
		{
			name: "structured version returns rules",
			version: models.PolicyVersion{
				PolicyVersion:  "p1",
				AuthoringMode:  store.AuthoringModeStructured,
				StructuredJSON: structuredJSON,
			},
			wantAdvancedMode: false,
		},
		{
			name:             "rego version returns advanced mode",
			version:          models.PolicyVersion{PolicyVersion: "p1", AuthoringMode: store.AuthoringModeRego},
			wantAdvancedMode: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
				Return(modelsAdminKey([]string{"admin:read"}), nil)
			storeMock.On("GetPolicyVersion", mock.Anything, "t1", "p1").Return(tc.version, nil)

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet,
				nil,
				testhelpers.Params{Names: []string{"tenant_id", "policy_version"}, Values: []string{"t1", "p1"}},
			)
			req.Header.Set(auth.AuthorizationHeader, "Bearer key")

			deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
			require.NoError(t, deps.handlePolicyStructuredGet(ctx))
			require.Equal(t, http.StatusOK, rec.Code)

			var resp PolicyStructuredResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, tc.wantAdvancedMode, resp.AdvancedMode)
			if tc.wantAdvancedMode {
				require.Nil(t, resp.Structured)
			} else {
				require.NotNil(t, resp.Structured)
				require.Len(t, resp.Structured.Rules, 1)
			}
		})
	}
}

func TestHandlePolicyCoverage(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetAdminKeyByHash", context.Background(), mock.Anything).
		Return(modelsAdminKey([]string{"admin:read"}), nil)
	storeMock.On("GetTenantConfig", mock.Anything, "t1").
		Return(models.TenantConfig{ActivePolicyVersion: "p1"}, nil)
	storeMock.On("ListFallbackHitPairs", mock.Anything, "t1", mock.Anything, mock.Anything, mock.Anything).
		Return([]models.CoverageFallbackHit{
			{ToolID: "jira", ActionType: "TICKET.CREATE", Decision: "DENY", RuleID: "rule_default_deny", HitCount: 5, LastSeen: time.Now().UTC()},
		}, nil)
	storeMock.On("ListUnusedActiveTools", mock.Anything, "t1").Return([]string{"unused_tool"}, nil)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.URL.RawQuery = "window_days=7&limit=100"
	req.Header.Set(auth.AuthorizationHeader, "Bearer key")

	deps := Dependencies{Store: storeMock, Metrics: newTestMetrics(), Config: config.Config{}}
	require.NoError(t, deps.handlePolicyCoverage(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	var report struct {
		ActivePolicyVersion string `json:"active_policy_version"`
		WindowDays          int    `json:"window_days"`
		Gaps                []struct {
			ToolID string `json:"tool_id"`
			Reason string `json:"reason"`
		} `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	require.Equal(t, "p1", report.ActivePolicyVersion)
	require.Equal(t, 7, report.WindowDays)
	require.Len(t, report.Gaps, 2)
	require.Equal(t, "jira", report.Gaps[0].ToolID)
}
