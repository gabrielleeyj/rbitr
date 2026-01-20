package public

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
)

const integrationPolicy = `package rbitr.policy

default decision := {"decision": "DENY", "rule_id": "rule_default", "reason": "default", "policy_version": "p_v1"}

decision := {"decision": "ALLOW", "rule_id": "rule_allow_refund", "reason": "ok", "policy_version": "p_v1"} if {
	input.action_type == "PAYMENT.REFUND"
} else := {"decision": "REQUIRE_APPROVAL", "rule_id": "rule_require_approval", "reason": "approval", "policy_version": "p_v1"} if {
	input.action_type == "ACCESS.ROLE_CHANGE"
} else := {"decision": "DENY", "rule_id": "rule_deny_export", "reason": "no export", "policy_version": "p_v1"} if {
	input.action_type == "DATA.EXPORT"
}
`

func TestHandleToolCall_ConnectorAndADR(t *testing.T) {
	called := make(chan struct{}, 1)
	toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refund" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		called <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer toolServer.Close()

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
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1`)).
				WithArgs(sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name"}).AddRow("t_demo", "Demo"))

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT policy_id, tenant_id, rego_module, policy_version, updated_at FROM rbitr.policies WHERE tenant_id = $1`)).
				WithArgs("t_demo").
				WillReturnRows(sqlmock.NewRows([]string{"policy_id", "tenant_id", "rego_module", "policy_version", "updated_at"}).
					AddRow("policy_demo", "t_demo", integrationPolicy, "p_v1", time.Now()))

			if tc.expectedDecision == "ALLOW" {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)).
					WithArgs("t_demo", "mock_internal").
					WillReturnRows(sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}).
						AddRow("mock_internal", "t_demo", toolServer.URL, "", ""))
			}

			if tc.expectApproval {
				mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.approval_requests (
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

			mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, reason, rule_id, policy_version, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`)).
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
				).
				WillReturnResult(sqlmock.NewResult(1, 1))

			storeAPI := store.New(db)
			policyEval := policy.NewEvaluator(storeAPI)
			restConnector := connector.NewREST(1024)

			deps := Dependencies{
				Store:     storeAPI,
				Policy:    policyEval,
				Connector: restConnector,
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
			require.Equal(t, tc.expectedStatus, rec.Code)

			var response ToolCallResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
			require.Equal(t, tc.expectedDecision, response.Decision)
			if tc.expectApproval {
				require.NotEmpty(t, response.ApprovalRequestID)
			}

			if tc.expectToolCall {
				select {
				case <-called:
					// ok
				default:
					t.Fatalf("expected connector to call tool server")
				}
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
