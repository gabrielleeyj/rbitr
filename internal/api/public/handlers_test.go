package public

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

func TestHandleToolCall(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
	tool := models.Tool{ToolID: "mock_internal", TenantID: "t1", BaseURL: "http://example", AuthType: "api_key", AuthValue: "key"}

	cases := []struct {
		name           string
		headers        map[string]string
		policyResult   policy.Result
		policyError    error
		storeSetup     func(*store.MockStoreAPI)
		connectorSetup func(*connector.MockConnector)
		expectedCode   int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:         "unauthorized",
			headers:      map[string]string{auth.AgentIDHeader: "agent"},
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "forbidden",
			headers:      map[string]string{auth.TenantKeyHeader: "key"},
			expectedCode: http.StatusForbidden,
		},
		{
			name:    "deny",
			headers: map[string]string{auth.TenantKeyHeader: "key", auth.AgentIDHeader: "agent"},
			policyResult: policy.Result{
				Version:       "2026-01-20",
				Decision:      "DENY",
				Risk:          "HIGH",
				Rule:          models.DecisionRule{ID: "rule_deny", Priority: 100},
				Reasons:       []models.DecisionReason{{Code: "DENY", Message: "nope"}},
				Constraints:   map[string]any{},
				PolicyVersion: "p_v1",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
				storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, mock.Anything).
					Return("", store.ErrNotFound)
				storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
					return record.Decision == "DENY" && record.ActionType == "PAYMENT.REFUND"
				})).Return(nil)
			},
			expectedCode: http.StatusForbidden,
		},
		{
			name:    "require approval",
			headers: map[string]string{auth.TenantKeyHeader: "key", auth.AgentIDHeader: "agent"},
			policyResult: policy.Result{
				Version:       "2026-01-20",
				Decision:      "REQUIRE_APPROVAL",
				Risk:          "HIGH",
				Rule:          models.DecisionRule{ID: "rule_approval", Priority: 50},
				Reasons:       []models.DecisionReason{{Code: "APPROVAL", Message: "approval"}},
				Constraints:   map[string]any{},
				PolicyVersion: "p_v1",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
				storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, mock.Anything).
					Return("", store.ErrNotFound)
				storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(900, nil)
				storeMock.On("InsertApprovalRequest", context.Background(), mock.MatchedBy(func(req models.ApprovalRequest) bool {
					path, _ := req.RequestContext["path"].(string)
					method, _ := req.RequestContext["http_method"].(string)
					_, hasBody := req.RequestContext["body_json"]
					return path == "/refund" && method == "POST" && hasBody
				})).Return(nil)
				storeMock.On("InsertADR", context.Background(), mock.Anything).Return(nil)
			},
			expectedCode: http.StatusConflict,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var payload ToolCallResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
				require.NotEmpty(t, payload.ApprovalRequestID)
				require.NotEmpty(t, payload.ApprovalToken)
				require.NotEmpty(t, payload.ExpiresAt)
				require.Equal(t, "approval_required", payload.Error)
			},
		},
		{
			name:    "approved execution",
			headers: map[string]string{auth.TenantKeyHeader: "key", auth.AgentIDHeader: "agent", approvalHeaderID: "ar1", approvalHeaderToken: "token123"},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				bodyHash := utils.HashBody([]byte("{}"))
				canonical := utils.CanonicalRequest{
					TenantID: "t1",
					AgentID:  "agent",
					ToolID:   "mock_internal",
					Method:   "POST",
					Path:     "/refund",
					Query:    "",
					Headers:  map[string]string{"content-type": "application/json"},
					BodyHash: bodyHash,
				}
				requestHash := utils.HashCanonical(&canonical)
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
				storeMock.On("GetApprovalForExecution", context.Background(), "t1", "ar1").Return(models.ApprovalRequest{
					ApprovalRequestID: "ar1",
					TenantID:          "t1",
					AgentID:           "agent",
					ToolID:            "mock_internal",
					ActionType:        "PAYMENT.REFUND",
					RequestHash:       requestHash,
					Status:            "APPROVED",
					ApprovalTokenHash: utils.HashString("token123"),
					ExpiresAt:         time.Now().UTC().Add(10 * time.Minute),
					PolicyVersion:     "p_v1",
					ActionSummary:     "Refund",
					Risk:              "MEDIUM",
					RuleID:            "rule_approval",
				}, nil)
				storeMock.On("ClaimApprovalExecution", context.Background(), "t1", "ar1", utils.HashString("token123"), requestHash, mock.Anything).
					Return(nil)
				storeMock.On("GetTool", context.Background(), tenant.TenantID, "mock_internal").Return(tool, nil)
				storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
					return record.Decision == "ALLOW" && record.ApprovalRequestID == "ar1"
				})).Return(nil)
				storeMock.On("MarkApprovalExecuted", context.Background(), "t1", "ar1", mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
			},
			connectorSetup: func(connMock *connector.MockConnector) {
				connMock.On("Execute", mock.Anything, mock.Anything).
					Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{"X-Test": "ok"}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var payload ToolCallResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
				require.Equal(t, "ALLOW", payload.Decision)
			},
		},
		{
			name:    "allow",
			headers: map[string]string{auth.TenantKeyHeader: "key", auth.AgentIDHeader: "agent"},
			policyResult: policy.Result{
				Version:       "2026-01-20",
				Decision:      "ALLOW",
				Risk:          "HIGH",
				Rule:          models.DecisionRule{ID: "rule_allow", Priority: 10},
				Reasons:       []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
				Constraints:   map[string]any{},
				PolicyVersion: "p_v1",
			},
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
				storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, mock.Anything).
					Return("", store.ErrNotFound)
				storeMock.On("GetTool", context.Background(), tenant.TenantID, "mock_internal").Return(tool, nil)
				storeMock.On("InsertADR", context.Background(), mock.Anything).Return(nil)
			},
			connectorSetup: func(connMock *connector.MockConnector) {
				connMock.On("Execute", mock.Anything, mock.Anything).
					Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{"X-Test": "ok"}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var payload ToolCallResponse
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
				require.Equal(t, "ok", payload.ToolBody)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			policyMock := policy.NewMockEvaluatorAPI(t)
			connectorMock := connector.NewMockConnector(t)
			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{
				Store:     storeAPI,
				Policy:    policyMock,
				Connector: connectorMock,
				Metrics:   newTestMetrics(),
				Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}
			if tc.connectorSetup != nil {
				tc.connectorSetup(connectorMock)
			}
			if tc.policyResult.Decision != "" || tc.policyError != nil {
				policyMock.On("Evaluate", context.Background(), "t1", mock.Anything).
					Return(tc.policyResult, tc.policyError)
			}

			payload := ToolCallRequest{
				HTTPMethod: "POST",
				Path:       "/refund",
				Query:      "",
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       "{}",
			}
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(payload),
				testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
			)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			err := deps.handleToolCall(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedCode, rec.Code)
			if tc.checkResponse != nil {
				tc.checkResponse(t, rec)
			}
		})
	}
}

func TestHandleToolCall_DenyShadowModeExecutes(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}
	tool := models.Tool{ToolID: "mock_internal", TenantID: "t1", BaseURL: "http://example", AuthType: "api_key", AuthValue: "key"}

	storeMock := store.NewMockStoreAPI(t)
	policyMock := policy.NewMockEvaluatorAPI(t)
	connectorMock := connector.NewMockConnector(t)

	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
	storeMock.On("GetRiskOverride", context.Background(), "t1", mock.Anything).Return("", store.ErrNotFound)
	storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
		TenantID:            "t1",
		ActivePolicyVersion: "p_v1",
		EnforcementMode:     enforcementModeShadow,
	}, nil)
	storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
		return record.Decision == decisionDeny && record.RuleID == "rule_deny"
	})).Return(nil)
	storeMock.On("GetTool", context.Background(), "t1", "mock_internal").Return(tool, nil)

	policyMock.On("Evaluate", context.Background(), "t1", mock.Anything).Return(policy.Result{
		Version:       "2026-01-20",
		Decision:      decisionDeny,
		Risk:          "HIGH",
		Rule:          models.DecisionRule{ID: "rule_deny", Priority: 100},
		Reasons:       []models.DecisionReason{{Code: "DENY", Message: "blocked"}},
		Constraints:   map[string]any{},
		PolicyVersion: "p_v1",
	}, nil)
	connectorMock.On("Execute", mock.Anything, mock.Anything).
		Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{"X-Test": "ok"}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

	deps := Dependencies{
		Store:     storeMock,
		Policy:    policyMock,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config: config.Config{
			BodyLimitSize:     256 * 1024,
			ResponseLimit:     256 * 1024,
			FeatureShadowMode: true,
		},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", rec.Header().Get(restShadowDenyHeader))

	var response ToolCallResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, decisionDeny, response.Decision)
	require.Equal(t, "ok", response.ToolBody)
	require.True(t, response.ShadowDenied)
	require.NotNil(t, response.RbitrShadow)
	require.Equal(t, decisionDeny, response.RbitrShadow["original_decision"])
}

func TestHandleToolCall_RequireApprovalStillEnforcedInShadowMode(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	storeMock := store.NewMockStoreAPI(t)
	policyMock := policy.NewMockEvaluatorAPI(t)
	connectorMock := connector.NewMockConnector(t)

	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
	storeMock.On("GetRiskOverride", context.Background(), "t1", mock.Anything).Return("", store.ErrNotFound)
	storeMock.On("GetTenantConfig", context.Background(), "t1").Return(models.TenantConfig{
		TenantID:            "t1",
		ActivePolicyVersion: "p_v1",
		EnforcementMode:     enforcementModeShadow,
	}, nil)
	storeMock.On("GetDefaultApprovalTTLSeconds", context.Background()).Return(900, nil)
	storeMock.On("InsertApprovalRequest", context.Background(), mock.Anything).Return(nil)
	storeMock.On("InsertADR", context.Background(), mock.Anything).Return(nil)

	policyMock.On("Evaluate", context.Background(), "t1", mock.Anything).Return(policy.Result{
		Version:       "2026-01-20",
		Decision:      "REQUIRE_APPROVAL",
		Risk:          "HIGH",
		Rule:          models.DecisionRule{ID: "rule_approval", Priority: 50},
		Reasons:       []models.DecisionReason{{Code: "APPROVAL", Message: "approval required"}},
		Constraints:   map[string]any{},
		PolicyVersion: "p_v1",
	}, nil)

	deps := Dependencies{
		Store:     storeMock,
		Policy:    policyMock,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config: config.Config{
			BodyLimitSize:     256 * 1024,
			ResponseLimit:     256 * 1024,
			FeatureShadowMode: true,
		},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Empty(t, rec.Header().Get(restShadowDenyHeader))
}

func TestHandleEvidence(t *testing.T) {
	cases := []struct {
		name         string
		headers      map[string]string
		pathTenant   string
		storeSetup   func(*store.MockStoreAPI)
		expectedCode int
	}{
		{
			name:         "unauthorized",
			headers:      map[string]string{auth.AgentIDHeader: "agent"},
			pathTenant:   "t1",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "tenant mismatch",
			headers:      map[string]string{auth.AgentIDHeader: "agent", auth.TenantKeyHeader: "key"},
			pathTenant:   "t2",
			expectedCode: http.StatusForbidden,
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
			},
		},
		{
			name:         "success",
			headers:      map[string]string{auth.AgentIDHeader: "agent", auth.TenantKeyHeader: "key"},
			pathTenant:   "t1",
			expectedCode: http.StatusOK,
			storeSetup: func(storeMock *store.MockStoreAPI) {
				storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
				storeMock.On("ListEvidence", context.Background(), "t1", 50).Return([]models.ActionDecisionRecord{
					{
						DecisionID:      "d1",
						RequestID:       "r1",
						TenantID:        "t1",
						AgentID:         "agent",
						ToolID:          "tool",
						ActionType:      "DATA.READ",
						ActionRisk:      "LOW",
						ActionSummary:   "Read data",
						Decision:        "ALLOW",
						DecisionVersion: "2026-01-20",
						DecisionRisk:    "LOW",
						RuleID:          "rule_allow",
						RulePriority:    10,
						Reasons:         []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
						Constraints:     map[string]any{},
						Tags:            []string{"tag"},
						PolicyVersion:   "p_v1",
						Reason:          "ok",
						RequestHash:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
						CreatedAt:       time.Now().UTC(),
					},
				}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeMock := store.NewMockStoreAPI(t)
			var storeAPI store.StoreAPI = storeMock
			deps := Dependencies{
				Store:   storeAPI,
				Metrics: newTestMetrics(),
				Config:  config.Config{BodyLimitSize: 256 * 1024},
			}
			if tc.storeSetup != nil {
				tc.storeSetup(storeMock)
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodGet,
				nil,
				testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{tc.pathTenant}},
			)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			err := deps.handleEvidence(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedCode, rec.Code)
			if tc.expectedCode == http.StatusOK {
				assertEvidenceWhitelist(t, rec.Body.Bytes())
			}
		})
	}
}

func TestHandleEvidenceTenantAuthBearerPreferred(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), utils.HashString("bearer_key")).
		Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
	storeMock.On("ListEvidence", context.Background(), "t1", 50).
		Return([]models.ActionDecisionRecord{}, nil)

	deps := Dependencies{
		Store:   storeMock,
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024},
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AuthorizationHeader, "Bearer bearer_key")
	req.Header.Set(auth.TenantKeyHeader, "legacy_key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleEvidence(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Deprecation"))
}

func TestHandleEvidenceTenantAuthFallbackSetsDeprecationHeaders(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), utils.HashString("legacy_key")).
		Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
	storeMock.On("ListEvidence", context.Background(), "t1", 50).
		Return([]models.ActionDecisionRecord{}, nil)

	deps := Dependencies{
		Store:   storeMock,
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024},
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "legacy_key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleEvidence(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", rec.Header().Get("Deprecation"))
	require.Equal(t, xTenantKeySunset, rec.Header().Get("Sunset"))
}

func TestHandleEvidenceTenantAuthFallbackDisabled(t *testing.T) {
	deps := Dependencies{
		Store:   store.NewMockStoreAPI(t),
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024, DisableXTenantKey: true},
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "legacy_key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleEvidence(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleEvidenceIncludesApprovalMetadata(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).
		Return(models.Tenant{TenantID: "t1", Enabled: true}, nil)
	storeMock.On("ListEvidence", context.Background(), "t1", 50).
		Return([]models.ActionDecisionRecord{
			{
				DecisionID:        "d1",
				RequestID:         "r1",
				TenantID:          "t1",
				AgentID:           "agent",
				ToolID:            "tool",
				ActionType:        "PAYMENT.REFUND",
				ActionRisk:        "HIGH",
				ActionSummary:     "Refund",
				Decision:          "ALLOW",
				DecisionVersion:   "2026-01-20",
				DecisionRisk:      "HIGH",
				RuleID:            "rule_allow",
				RulePriority:      10,
				Reasons:           []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
				Constraints:       map[string]any{},
				PolicyVersion:     "p_v1",
				Reason:            "ok",
				RequestHash:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
				ApprovalRequestID: "ar1",
				CreatedAt:         time.Now().UTC(),
			},
		}, nil)
	storeMock.On("GetApprovalRequest", context.Background(), "t1", "ar1").
		Return(models.ApprovalRequest{
			ApprovalRequestID:  "ar1",
			TenantID:           "t1",
			AgentID:            "agent",
			ToolID:             "tool",
			ActionType:         "PAYMENT.REFUND",
			RequestHash:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			Status:             "EXECUTED",
			ExpiresAt:          time.Now().UTC().Add(10 * time.Minute),
			CreatedAt:          time.Now().UTC(),
			PolicyVersion:      "p_v1",
			DecidedAt:          ptrTime(time.Now().UTC()),
			DecidedBy:          "admin",
			DecisionComment:    "approved",
			ExecutedAt:         ptrTime(time.Now().UTC()),
			ExecutedRequestID:  "req_exec",
			ExecutedDecisionID: "dec_exec",
			RequestDecisionID:  "dec_req",
			ActionSummary:      "Refund",
			Risk:               "HIGH",
			RuleID:             "rule_approval",
			Reasons:            []models.DecisionReason{{Code: "APPROVED", Message: "ok"}},
		}, nil)

	var storeAPI store.StoreAPI = storeMock
	deps := Dependencies{
		Store:   storeAPI,
		Metrics: newTestMetrics(),
		Config:  config.Config{BodyLimitSize: 256 * 1024},
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodGet,
		nil,
		testhelpers.Params{Names: []string{"tenant_id"}, Values: []string{"t1"}},
	)
	req.Header.Set(auth.AgentIDHeader, "agent")
	req.Header.Set(auth.TenantKeyHeader, "key")

	err := deps.handleEvidence(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload EvidenceResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
	require.Len(t, payload.Records, 1)
	record := payload.Records[0]
	require.Equal(t, "EXECUTED", record.ApprovalStatus)
	require.Equal(t, "admin", record.ApprovalDecidedBy)
	require.Equal(t, "approved", record.ApprovalComment)
	require.Equal(t, "req_exec", record.ApprovalExecutedRequestID)
	require.Equal(t, "dec_exec", record.ApprovalExecutedDecisionID)
	require.Equal(t, "dec_req", record.ApprovalRequestDecisionID)
	require.NotNil(t, record.ApprovalDecidedAt)
	require.NotNil(t, record.ApprovalExecutedAt)
}

func newTestMetrics() *telemetry.Metrics {
	return &telemetry.Metrics{
		DecisionsTotal:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_decisions_total"}, []string{"decision", "action_type"}),
		GatewayRequests:         prometheus.NewCounter(prometheus.CounterOpts{Name: "test_gateway_requests_total"}),
		ToolExecTotal:           prometheus.NewCounter(prometheus.CounterOpts{Name: "test_tool_exec_total"}),
		ErrorsTotal:             prometheus.NewCounter(prometheus.CounterOpts{Name: "test_errors_total"}),
		DecisionLatencyMs:       prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_decision_latency_ms"}),
		ToolLatencyMs:           prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_tool_latency_ms"}),
		PolicyEvalInvalidTotal:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_policy_eval_invalid_total"}, []string{"reason"}),
		ApprovalsCreatedTotal:   prometheus.NewCounter(prometheus.CounterOpts{Name: "test_approvals_created_total"}),
		ApprovalsResolvedTotal:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_approvals_resolved_total"}, []string{"resolution"}),
		ApprovalsExecuteTotal:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_approvals_execute_total"}, []string{"result"}),
		CacheHitsTotal:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_cache_hits_total"}, []string{"cache"}),
		CacheMissesTotal:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_cache_misses_total"}, []string{"cache"}),
		TenantAuthFallbackTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "test_tenant_auth_fallback_total"}),
		TenantKeyLegacyUpgradeTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_tenant_key_legacy_upgrade_total",
		}),
		RateLimitChecksTotal:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_rate_limit_checks_total"}, []string{"result", "window"}),
		RateLimitExceededTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_rate_limit_exceeded_total"}, []string{"window", "scope"}),
		RateLimitLatencyMs:     prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_rate_limit_latency_ms"}),
	}
}

func TestHandleToolCallClassification(t *testing.T) {
	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
	}
	classificationResult := classification.Classify("mock_internal", payload.HTTPMethod, payload.Path, payload.Query, payload.Headers)
	require.Equal(t, "PAYMENT.REFUND", classificationResult.ActionType)
	require.Equal(t, classification.RiskHigh, classificationResult.ActionRisk)
}

func TestHandleToolCallRateLimitExceeded(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
	storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, "PAYMENT.REFUND").
		Return("", store.ErrNotFound)
	storeMock.On("GetEffectiveRateLimitConfig", context.Background(), tenant.TenantID).
		Return(models.RateLimitConfig{
			PerMinute: 1,
			PerDay:    10000,
			Scope:     "tenant_agent_tool",
		}, nil)
	storeMock.On(
		"IncrementRateLimitCounter",
		context.Background(),
		tenant.TenantID,
		"agent",
		"mock_internal",
		"",
		"minute",
		mock.Anything,
		mock.Anything,
		int64(1),
	).Return(false, int64(0), nil)
	storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
		return record.Decision == decisionDeny &&
			record.RuleID == "rate_limit_minute" &&
			len(record.Reasons) == 1 &&
			record.Reasons[0].Code == "RATE_LIMIT_EXCEEDED"
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.On("Evaluate", context.Background(), tenant.TenantID, mock.Anything).Return(policy.Result{
		Version:       "2026-01-20",
		Decision:      "ALLOW",
		Risk:          "HIGH",
		Rule:          models.DecisionRule{ID: "rule_allow", Priority: 10},
		Reasons:       []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
		Constraints:   map[string]any{},
		PolicyVersion: "p_v1",
	}, nil)

	connectorMock := connector.NewMockConnector(t)
	var storeAPI store.StoreAPI = storeMock
	deps := Dependencies{
		Store:     storeAPI,
		Policy:    policyMock,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config: config.Config{
			BodyLimitSize:       256 * 1024,
			ResponseLimit:       256 * 1024,
			FeatureRateLimiting: true,
		},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	var response map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.Equal(t, "RATE_LIMIT_EXCEEDED", response["code"])
	require.Equal(t, "minute", response["window"])

	connectorMock.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}

func TestHandleToolCallArgConstraintDenied(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
	storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, "PAYMENT.REFUND").
		Return("", store.ErrNotFound)
	storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
		failures, ok := record.Constraints["arg_constraint_failures"].([]map[string]any)
		return record.Decision == decisionDeny &&
			record.RuleID == "deny_prod_branch" &&
			len(record.Reasons) == 1 &&
			record.Reasons[0].Code == argConstraintReasonDeny &&
			ok &&
			len(failures) == 1
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.On("Evaluate", context.Background(), tenant.TenantID, mock.Anything).Return(policy.Result{
		Version:  "2026-01-20",
		Decision: "ALLOW",
		Risk:     "HIGH",
		Rule:     models.DecisionRule{ID: "rule_allow", Priority: 10},
		Reasons:  []models.DecisionReason{{Code: "ALLOW", Message: "ok"}},
		Constraints: map[string]any{
			"args": map[string]any{
				"deny": []any{
					map[string]any{
						"id":      "deny_prod_branch",
						"path":    "/branch",
						"op":      "prefix",
						"value":   "prod/",
						"message": "branch blocked",
					},
				},
			},
		},
		PolicyVersion: "p_v1",
	}, nil)

	connectorMock := connector.NewMockConnector(t)
	var storeAPI store.StoreAPI = storeMock
	deps := Dependencies{
		Store:     storeAPI,
		Policy:    policyMock,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config: config.Config{
			BodyLimitSize:         256 * 1024,
			ResponseLimit:         256 * 1024,
			FeatureArgConstraints: true,
			FeatureRateLimiting:   false,
		},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"branch":"prod/release"}`,
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)

	var response ToolCallResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.Equal(t, decisionDeny, response.Decision)
	require.Equal(t, "branch blocked", response.Reason)

	connectorMock.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}

func TestHandleToolCallPersistsMatchedRules(t *testing.T) {
	tenant := models.Tenant{TenantID: "t1", Name: "Tenant", Enabled: true}

	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantByKeyHash", context.Background(), mock.Anything).Return(tenant, nil)
	storeMock.On("GetRiskOverride", context.Background(), tenant.TenantID, "PAYMENT.REFUND").
		Return("", store.ErrNotFound)
	storeMock.On("InsertADR", context.Background(), mock.MatchedBy(func(record models.ActionDecisionRecord) bool {
		rawMatched, ok := record.Constraints["matched_rules"]
		if !ok {
			return false
		}
		matched, ok := rawMatched.([]map[string]any)
		if !ok || len(matched) != 2 {
			return false
		}
		firstRuleID, _ := matched[0]["rule_id"].(string)
		secondRuleID, _ := matched[1]["rule_id"].(string)
		return record.Decision == decisionDeny &&
			firstRuleID == "rule_deny_high" &&
			secondRuleID == "rule_allow_low"
	})).Return(nil)

	policyMock := policy.NewMockEvaluatorAPI(t)
	policyMock.On("Evaluate", context.Background(), tenant.TenantID, mock.Anything).Return(policy.Result{
		Version:  "2026-01-20",
		Decision: decisionDeny,
		Risk:     "HIGH",
		Rule:     models.DecisionRule{ID: "rule_deny_high", Priority: 100},
		Reasons:  []models.DecisionReason{{Code: "DENY", Message: "blocked"}},
		Constraints: map[string]any{
			"rate_limit": map[string]any{"per_minute": float64(60)},
		},
		MatchedRules: []models.DecisionMatchedRule{
			{RuleID: "rule_deny_high", Priority: 100, Effect: decisionDeny},
			{RuleID: "rule_allow_low", Priority: 10, Effect: "ALLOW"},
		},
		PolicyVersion: "p_v1",
	}, nil)

	connectorMock := connector.NewMockConnector(t)
	var storeAPI store.StoreAPI = storeMock
	deps := Dependencies{
		Store:     storeAPI,
		Policy:    policyMock,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config: config.Config{
			BodyLimitSize:       256 * 1024,
			ResponseLimit:       256 * 1024,
			FeatureRateLimiting: false,
		},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set(auth.TenantKeyHeader, "key")
	req.Header.Set(auth.AgentIDHeader, "agent")

	err := deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGetToolCachedUsesTenantConfigVersion(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 1}, nil).Once()
	storeMock.On("GetTool", context.Background(), "t1", "tool1").
		Return(models.Tool{TenantID: "t1", ToolID: "tool1", BaseURL: "http://v1"}, nil).Once()
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 1}, nil).Once()
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 2}, nil).Once()
	storeMock.On("GetTool", context.Background(), "t1", "tool1").
		Return(models.Tool{TenantID: "t1", ToolID: "tool1", BaseURL: "http://v2"}, nil).Once()

	deps := Dependencies{
		Store:     storeMock,
		Metrics:   newTestMetrics(),
		ToolCache: cache.New[models.Tool](time.Minute),
	}

	toolV1, err := deps.getToolCached(context.Background(), "t1", "tool1")
	require.NoError(t, err)
	require.Equal(t, "http://v1", toolV1.BaseURL)

	toolV1Cached, err := deps.getToolCached(context.Background(), "t1", "tool1")
	require.NoError(t, err)
	require.Equal(t, "http://v1", toolV1Cached.BaseURL)

	toolV2, err := deps.getToolCached(context.Background(), "t1", "tool1")
	require.NoError(t, err)
	require.Equal(t, "http://v2", toolV2.BaseURL)
}

func TestGetRiskOverrideCachedUsesTenantConfigVersion(t *testing.T) {
	storeMock := store.NewMockStoreAPI(t)
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 1}, nil).Once()
	storeMock.On("GetRiskOverride", context.Background(), "t1", "DATA.EXPORT").
		Return("HIGH", nil).Once()
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 1}, nil).Once()
	storeMock.On("GetTenantConfig", context.Background(), "t1").
		Return(models.TenantConfig{TenantID: "t1", Version: 2}, nil).Once()
	storeMock.On("GetRiskOverride", context.Background(), "t1", "DATA.EXPORT").
		Return("CRITICAL", nil).Once()

	deps := Dependencies{
		Store:             storeMock,
		Metrics:           newTestMetrics(),
		RiskOverrideCache: cache.New[string](time.Minute),
	}

	riskV1, err := deps.getRiskOverrideCached(context.Background(), "t1", "DATA.EXPORT")
	require.NoError(t, err)
	require.Equal(t, "HIGH", riskV1)

	riskV1Cached, err := deps.getRiskOverrideCached(context.Background(), "t1", "DATA.EXPORT")
	require.NoError(t, err)
	require.Equal(t, "HIGH", riskV1Cached)

	riskV2, err := deps.getRiskOverrideCached(context.Background(), "t1", "DATA.EXPORT")
	require.NoError(t, err)
	require.Equal(t, "CRITICAL", riskV2)
}

func assertEvidenceWhitelist(t *testing.T, body []byte) {
	validateEvidenceContract(t, body)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))

	require.ElementsMatch(t, []string{"tenant_id", "records"}, keys(payload))

	records, ok := payload["records"].([]any)
	require.True(t, ok)
	require.Len(t, records, 1)

	record, ok := records[0].(map[string]any)
	require.True(t, ok)

	allowed := map[string]bool{
		"decision_id":                   true,
		"request_id":                    true,
		"tenant_id":                     true,
		"agent_id":                      true,
		"tool_id":                       true,
		"action_type":                   true,
		"action_risk":                   true,
		"action_summary":                true,
		"decision":                      true,
		"decision_version":              true,
		"decision_risk":                 true,
		"rule_id":                       true,
		"rule_priority":                 true,
		"reasons":                       true,
		"constraints":                   true,
		"tags":                          true,
		"policy_version":                true,
		"reason":                        true,
		"request_hash":                  true,
		"response_hash":                 true,
		"approval_request_id":           true,
		"approval_status":               true,
		"approval_decided_at":           true,
		"approval_decided_by":           true,
		"approval_decision_comment":     true,
		"approval_executed_at":          true,
		"approval_executed_request_id":  true,
		"approval_executed_decision_id": true,
		"approval_request_decision_id":  true,
		"timestamp":                     true,
	}

	for key := range record {
		if !allowed[key] {
			t.Fatalf("unexpected field in evidence export: %s", key)
		}
	}

	serialized := string(body)
	for _, term := range []string{"password", "authorization", "ssn", "raw", "payload", "request_body"} {
		require.False(t, strings.Contains(serialized, term))
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
