package public

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

const integrationPolicy = `package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
	"version": "2026-01-20",
	"decision": decision,
	"risk": risk,
	"rule": {"id": rule_id, "priority": priority},
	"reasons": [{"code": code, "message": message}],
	"constraints": {}
} if { true }

default decision := {
	"version": "2026-01-20",
	"decision": "DENY",
	"risk": "LOW",
	"rule": {"id": "rule_default", "priority": 100},
	"reasons": [{"code": "DEFAULT_DENY", "message": "default"}],
	"constraints": {}
}

decision := decision_obj("ALLOW", input.action_risk, "rule_allow_refund", 10, "ALLOW", "ok") if {
	input.action_type == "PAYMENT.REFUND"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval", 50, "APPROVAL", "approval") if {
	input.action_type == "ACCESS.ROLE_CHANGE"
} else := decision_obj("DENY", input.action_risk, "rule_deny_export", 100, "DENY_EXPORT", "no export") if {
	input.action_type == "DATA.EXPORT"
}
`

func TestHandleToolCall_ConnectorAndADR(t *testing.T) {
	cases := []struct {
		name             string
		path             string
		method           string
		expectedStatus   int
		expectedDecision string
		expectApproval   bool
		expectToolCall   bool
	}{
		{
			name:             "allow",
			path:             "/refund",
			method:           http.MethodPost,
			expectedStatus:   http.StatusOK,
			expectedDecision: "ALLOW",
			expectToolCall:   true,
		},
		{
			name:             "require approval",
			path:             "/change_role",
			method:           http.MethodPost,
			expectedStatus:   http.StatusConflict,
			expectedDecision: "REQUIRE_APPROVAL",
			expectApproval:   true,
		},
		{
			name:             "deny",
			path:             "/export_customer_data",
			method:           http.MethodPost,
			expectedStatus:   http.StatusForbidden,
			expectedDecision: "DENY",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, sm, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			sm.MatchExpectationsInOrder(false)

			sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1`)).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name"}).AddRow("t_demo", "Demo"))

			sm.ExpectQuery(regexp.QuoteMeta(`SELECT action_risk FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`)).
				WithArgs("t_demo", sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"action_risk"}))

			sm.ExpectQuery(regexp.QuoteMeta(`SELECT policy_id, tenant_id, rego_module, policy_version, updated_at FROM rbitr.policies WHERE tenant_id = $1`)).
				WithArgs("t_demo").
				WillReturnRows(sqlmock.NewRows([]string{"policy_id", "tenant_id", "rego_module", "policy_version", "updated_at"}).
					AddRow("policy_demo", "t_demo", integrationPolicy, "p_v1", time.Now()))
			sm.ExpectQuery(regexp.QuoteMeta(`SELECT policy_id, tenant_id, rego_module, policy_version, updated_at FROM rbitr.policies WHERE tenant_id = $1`)).
				WithArgs("t_demo").
				WillReturnRows(sqlmock.NewRows([]string{"policy_id", "tenant_id", "rego_module", "policy_version", "updated_at"}).
					AddRow("policy_demo", "t_demo", integrationPolicy, "p_v1", time.Now()))

			if tc.expectedDecision == "ALLOW" {
				sm.ExpectQuery(regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)).
					WithArgs("t_demo", "mock_internal").
					WillReturnRows(sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}).
						AddRow("mock_internal", "t_demo", "http://mock.local", "", ""))
			}

			if tc.expectApproval {
				sm.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.approval_requests (
		approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash,
		status, expires_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`)).
					WithArgs(
						sqlmock.AnyArg(),
						"t_demo",
						"agent_demo",
						"mock_internal",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"PENDING",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			sm.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`)).
				WithArgs(
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					"t_demo",
					"agent_demo",
					"mock_internal",
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					tc.expectedDecision,
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
				).
				WillReturnResult(sqlmock.NewResult(1, 1))

			storeAPI := store.New(db)
			policyEval := policy.NewEvaluator(storeAPI)
			classificationResult := classification.Classify("mock_internal", tc.method, tc.path, "", map[string]string{"Content-Type": "application/json"})
			if _, evalErr := policyEval.Evaluate(context.Background(), "t_demo", map[string]any{
				"action_type": classificationResult.ActionType,
				"action_risk": classificationResult.ActionRisk,
			}); evalErr != nil {
				t.Fatalf("policy eval preflight failed: %v", evalErr)
			}
			connectorMock := connector.NewMockConnector(t)
			if tc.expectToolCall {
				connectorMock.On("Execute", mock.Anything, mock.MatchedBy(func(req connector.Request) bool {
					return req.Method == tc.method && req.URL == "http://mock.local"+tc.path
				})).Return(connector.Response{
					Status:   http.StatusOK,
					Headers:  map[string]string{"Content-Type": "application/json"},
					Body:     []byte(`{"status":"ok"}`),
					BodyHash: "hash",
				}, nil)
			}

			deps := Dependencies{
				Store:     storeAPI,
				Policy:    policyEval,
				Connector: connectorMock,
				Metrics:   newTestMetrics(),
				Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 1024},
			}

			payload := ToolCallRequest{
				HTTPMethod: tc.method,
				Path:       tc.path,
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       "{}",
			}
			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(payload),
				testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
			)
			req.Header.Set("X-Tenant-Key", "tenant_demo_key")
			req.Header.Set("X-Agent-Id", "agent_demo")

			err = deps.handleToolCall(ctx)
			require.NoError(t, err)
			if rec.Code != tc.expectedStatus {
				t.Fatalf("expected status %d got %d body=%s", tc.expectedStatus, rec.Code, rec.Body.String())
			}

			var response ToolCallResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
			require.Equal(t, tc.expectedDecision, response.Decision)
			if tc.expectApproval {
				require.NotEmpty(t, response.ApprovalRequestID)
			}

			if tc.expectToolCall {
				connectorMock.AssertCalled(t, "Execute", mock.Anything, mock.Anything)
			} else {
				connectorMock.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
			}

			require.NoError(t, sm.ExpectationsWereMet())
		})
	}
}
