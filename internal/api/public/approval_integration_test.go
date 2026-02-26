package public

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adminapi "github.com/gabrielleeyj/rbitr/internal/api/admin"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

func TestHandleToolCallApprovedExecutionIntegration(t *testing.T) {
	db, sm, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tenantKeyHash := utils.HashString("tenant_demo_key")
	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)).
		WithArgs(tenantKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t_demo", "Demo", true))

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/refund",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	bodyHash := utils.HashBody([]byte(payload.Body))
	canonical := utils.CanonicalRequest{
		TenantID: "t_demo",
		AgentID:  "agent_demo",
		ToolID:   "mock_internal",
		Method:   payload.HTTPMethod,
		Path:     payload.Path,
		Query:    payload.Query,
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, action_summary, risk, rule_id, request_context, reasons,
		executing_at, execution_id, failed_at, last_error_code
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", "ar_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "action_summary", "risk", "rule_id", "request_context", "reasons",
			"executing_at", "execution_id", "failed_at", "last_error_code",
		}).AddRow(
			"ar_1", "t_demo", "agent_demo", "mock_internal", "PAYMENT.REFUND", requestHash, "APPROVED",
			utils.HashString("token123"), time.Now().UTC().Add(5*time.Minute), time.Now().UTC(), "p_v1", "Refund", "MEDIUM", "rule_approval", nil, nil,
			nil, nil, nil, nil,
		))

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXECUTING', executing_at = $1, execution_id = COALESCE(execution_id, approval_request_id), last_error_code = NULL
		WHERE tenant_id = $2
			AND approval_request_id = $3
			AND status = 'APPROVED'
			AND expires_at > $4
			AND approval_token_hash = $5
			AND request_hash = $6`)).
		WithArgs(sqlmock.AnyArg(), "t_demo", "ar_1", sqlmock.AnyArg(), utils.HashString("token123"), requestHash).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)).
		WithArgs("t_demo", "mock_internal").
		WillReturnRows(sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}).
			AddRow("mock_internal", "t_demo", "http://mock.local", "", "", "http", nil, nil, nil))

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXECUTED', executed_at = $1, executed_request_id = $2, executed_decision_id = $3, last_error_code = NULL
		WHERE tenant_id = $4 AND approval_request_id = $5 AND status = 'EXECUTING'`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t_demo", "ar_1").
		WillReturnResult(sqlmock.NewResult(1, 1))

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
			"ALLOW",
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
	connectorMock := connector.NewMockConnector(t)
	connectorMock.On("Execute", mock.Anything, mock.Anything).
		Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{"X-Test": "ok"}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

	deps := Dependencies{
		Store:     storeAPI,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
	}

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set("X-Tenant-Key", "tenant_demo_key")
	req.Header.Set("X-Agent-Id", "agent_demo")
	req.Header.Set(approvalHeaderID, "ar_1")
	req.Header.Set(approvalHeaderToken, "token123")

	err = deps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, sm.ExpectationsWereMet())
}

func TestApprovalFlowEndToEnd(t *testing.T) {
	db, sm, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tenantKeyHash := utils.HashString("tenant_demo_key")
	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)).
		WithArgs(tenantKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t_demo", "Demo", true))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT action_risk FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`)).
		WithArgs("t_demo", "ACCESS.ROLE_CHANGE").
		WillReturnError(sql.ErrNoRows)

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT pv.policy_version, pv.tenant_id, pv.rego_module, pv.created_at
		FROM rbitr.tenant_config tc
		JOIN rbitr.policy_versions pv
			ON pv.tenant_id = tc.tenant_id
			AND pv.policy_version = tc.active_policy_version
		WHERE tc.tenant_id = $1`)).
		WithArgs("t_demo").
		WillReturnRows(sqlmock.NewRows([]string{"policy_version", "tenant_id", "rego_module", "created_at"}).
			AddRow("p_v1", "t_demo", integrationPolicy, time.Now()))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs("default_approval_ttl_seconds").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("900"))

	sm.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.approval_requests (
		approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash,
		status, approval_token_hash, expires_at, created_at, policy_version,
		decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id,
		request_decision_id, action_summary, risk, rule_id, request_context, reasons
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`)).
		WithArgs(
			sqlmock.AnyArg(),
			"t_demo",
			"agent_demo",
			"mock_internal",
			"ACCESS.ROLE_CHANGE",
			sqlmock.AnyArg(),
			"PENDING",
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
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

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
			"ACCESS.ROLE_CHANGE",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"REQUIRE_APPROVAL",
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
	connectorMock := connector.NewMockConnector(t)
	connectorMock.On("Execute", mock.Anything, mock.Anything).
		Return(connector.Response{Status: http.StatusOK, Headers: map[string]string{"X-Test": "ok"}, Body: []byte("ok"), BodyHash: "sha256:abc"}, nil)

	publicDeps := Dependencies{
		Store:     storeAPI,
		Policy:    policyEval,
		Connector: connectorMock,
		Metrics:   newTestMetrics(),
		Config:    config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
	}

	payload := ToolCallRequest{
		HTTPMethod: "POST",
		Path:       "/change_role",
		Query:      "",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       "{}",
	}
	bodyHash := utils.HashBody([]byte(payload.Body))
	canonical := utils.CanonicalRequest{
		TenantID: "t_demo",
		AgentID:  "agent_demo",
		ToolID:   "mock_internal",
		Method:   payload.HTTPMethod,
		Path:     payload.Path,
		Query:    payload.Query,
		Headers:  map[string]string{"content-type": "application/json"},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)

	ctx, req, rec := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req.Header.Set("X-Tenant-Key", "tenant_demo_key")
	req.Header.Set("X-Agent-Id", "agent_demo")

	err = publicDeps.handleToolCall(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code)

	var approvalResp ToolCallResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&approvalResp))
	require.NotEmpty(t, approvalResp.ApprovalRequestID)
	require.NotEmpty(t, approvalResp.ApprovalToken)

	adminKeyHash := utils.HashString("admin_demo_key")
	sm.ExpectQuery(regexp.QuoteMeta(`SELECT admin_key_id, key_hash, scopes FROM rbitr.admin_keys WHERE key_hash = $1`)).
		WithArgs(adminKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"admin_key_id", "key_hash", "scopes"}).
			AddRow("admin_demo", adminKeyHash, "{admin:read,admin:write}"))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, tc.active_policy_version, COALESCE(tool_counts.tool_count, 0)
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN (
			SELECT tenant_id, COUNT(*) AS tool_count
			FROM rbitr.tools
			GROUP BY tenant_id
		) tool_counts ON tool_counts.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1
		  AND t.deleted_at IS NULL`)).
		WithArgs("t_demo").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}).
			AddRow("t_demo", "Demo", "p_v1", 1))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT admin_key_id, key_hash, scopes FROM rbitr.admin_keys WHERE key_hash = $1`)).
		WithArgs(adminKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"admin_key_id", "key_hash", "scopes"}).
			AddRow("admin_demo", adminKeyHash, "{admin:read,admin:write}"))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by", "decision_comment",
			"executed_at", "executed_request_id", "executed_decision_id", "request_decision_id", "action_summary",
			"risk", "rule_id", "request_context", "reasons",
		}).AddRow(
			approvalResp.ApprovalRequestID, "t_demo", "agent_demo", "mock_internal", "ACCESS.ROLE_CHANGE", requestHash, "PENDING",
			utils.HashString(approvalResp.ApprovalToken), time.Now().UTC().Add(10*time.Minute), time.Now().UTC(), "p_v1", nil, nil, nil,
			nil, nil, nil, "dec_req", "Change role", "HIGH", "rule_require_approval", nil, nil,
		))

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = $1, decided_at = $2, decided_by = $3, decision_comment = $4
		WHERE tenant_id = $5 AND approval_request_id = $6 AND status = 'PENDING'`)).
		WithArgs("APPROVED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t_demo", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by", "decision_comment",
			"executed_at", "executed_request_id", "executed_decision_id", "request_decision_id", "action_summary",
			"risk", "rule_id", "request_context", "reasons",
		}).AddRow(
			approvalResp.ApprovalRequestID, "t_demo", "agent_demo", "mock_internal", "ACCESS.ROLE_CHANGE", requestHash, "APPROVED",
			utils.HashString(approvalResp.ApprovalToken), time.Now().UTC().Add(10*time.Minute), time.Now().UTC(), "p_v1", time.Now().UTC(), "admin_demo", "ok",
			nil, nil, nil, "dec_req", "Change role", "HIGH", "rule_require_approval", nil, nil,
		))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT event_hash
		FROM rbitr.admin_audit_events
		WHERE stream_id = $1 AND event_hash IS NOT NULL
		ORDER BY created_at DESC, audit_event_id DESC
		LIMIT 1`)).
		WithArgs("t_demo").
		WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))

	sm.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`)).
		WithArgs(
			sqlmock.AnyArg(),
			"t_demo",
			"t_demo",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"admin_key",
			"admin_demo",
			"admin_demo",
			sqlmock.AnyArg(),
			"APPROVAL.REQUEST",
			approvalResp.ApprovalRequestID,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	adminDeps := adminapi.Dependencies{
		Store:   storeAPI,
		Metrics: newTestMetrics(),
		Config:  config.Config{},
	}
	adminEcho := echo.New()
	adminapi.RegisterRoutes(adminEcho, &adminDeps)
	adminReq := httptest.NewRequest(
		http.MethodPost,
		"/admin/tenants/t_demo/approvals/"+approvalResp.ApprovalRequestID+"/approve",
		testhelpers.MakeBody(adminapi.ApprovalDecisionRequest{Comment: "ok"}),
	)
	adminReq.Header.Set("Content-Type", "application/json")
	adminReq.Header.Set("Authorization", "Bearer admin_demo_key")
	adminRec := httptest.NewRecorder()
	adminEcho.ServeHTTP(adminRec, adminReq)
	require.Equal(t, http.StatusOK, adminRec.Code)

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)).
		WithArgs(tenantKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t_demo", "Demo", true))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, action_summary, risk, rule_id, request_context, reasons,
		executing_at, execution_id, failed_at, last_error_code
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", approvalResp.ApprovalRequestID).
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "action_summary", "risk", "rule_id", "request_context", "reasons",
			"executing_at", "execution_id", "failed_at", "last_error_code",
		}).AddRow(
			approvalResp.ApprovalRequestID, "t_demo", "agent_demo", "mock_internal", "ACCESS.ROLE_CHANGE", requestHash, "APPROVED",
			utils.HashString(approvalResp.ApprovalToken), time.Now().UTC().Add(10*time.Minute), time.Now().UTC(), "p_v1", "Change role", "HIGH", "rule_require_approval", nil, nil,
			nil, nil, nil, nil,
		))

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXECUTING', executing_at = $1, execution_id = COALESCE(execution_id, approval_request_id), last_error_code = NULL
		WHERE tenant_id = $2
			AND approval_request_id = $3
			AND status = 'APPROVED'
			AND expires_at > $4
			AND approval_token_hash = $5
			AND request_hash = $6`)).
		WithArgs(sqlmock.AnyArg(), "t_demo", approvalResp.ApprovalRequestID, sqlmock.AnyArg(), utils.HashString(approvalResp.ApprovalToken), requestHash).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)).
		WithArgs("t_demo", "mock_internal").
		WillReturnRows(sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}).
			AddRow("mock_internal", "t_demo", "http://mock.local", "", "", "http", nil, nil, nil))

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXECUTED', executed_at = $1, executed_request_id = $2, executed_decision_id = $3, last_error_code = NULL
		WHERE tenant_id = $4 AND approval_request_id = $5 AND status = 'EXECUTING'`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t_demo", approvalResp.ApprovalRequestID).
		WillReturnResult(sqlmock.NewResult(1, 1))

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
			"ACCESS.ROLE_CHANGE",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"ALLOW",
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

	ctx2, req2, rec2 := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req2.Header.Set("X-Tenant-Key", "tenant_demo_key")
	req2.Header.Set("X-Agent-Id", "agent_demo")
	req2.Header.Set(approvalHeaderID, approvalResp.ApprovalRequestID)
	req2.Header.Set(approvalHeaderToken, approvalResp.ApprovalToken)

	err = publicDeps.handleToolCall(ctx2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec2.Code)

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)).
		WithArgs(tenantKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t_demo", "Demo", true))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, action_summary, risk, rule_id, request_context, reasons,
		executing_at, execution_id, failed_at, last_error_code
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", approvalResp.ApprovalRequestID).
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "action_summary", "risk", "rule_id", "request_context", "reasons",
			"executing_at", "execution_id", "failed_at", "last_error_code",
		}).AddRow(
			approvalResp.ApprovalRequestID, "t_demo", "agent_demo", "mock_internal", "ACCESS.ROLE_CHANGE", requestHash, "EXECUTED",
			utils.HashString(approvalResp.ApprovalToken), time.Now().UTC().Add(10*time.Minute), time.Now().UTC(), "p_v1", "Change role", "HIGH", "rule_require_approval", nil, nil,
			nil, approvalResp.ApprovalRequestID, nil, nil,
		))

	ctx3, req3, rec3 := testhelpers.MakeRequestWithParams(
		http.MethodPost,
		testhelpers.MakeBody(payload),
		testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
	)
	req3.Header.Set("X-Tenant-Key", "tenant_demo_key")
	req3.Header.Set("X-Agent-Id", "agent_demo")
	req3.Header.Set(approvalHeaderID, approvalResp.ApprovalRequestID)
	req3.Header.Set(approvalHeaderToken, approvalResp.ApprovalToken)

	err = publicDeps.handleToolCall(ctx3)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec3.Code)

	require.NoError(t, sm.ExpectationsWereMet())
}

func TestApprovalResubmissionInvalidCases(t *testing.T) {
	cases := []struct {
		name             string
		requestBody      string
		approvalStatus   string
		expiresAt        time.Time
		token            string
		expectedStatus   int
		expectedError    string
		expectExpireExec bool
	}{
		{
			name:           "wrong token",
			requestBody:    "{}",
			approvalStatus: "APPROVED",
			expiresAt:      time.Now().UTC().Add(5 * time.Minute),
			token:          "wrong",
			expectedStatus: http.StatusForbidden,
			expectedError:  "approval_token_invalid",
		},
		{
			name:           "hash mismatch",
			requestBody:    "{\"amount\":200}",
			approvalStatus: "APPROVED",
			expiresAt:      time.Now().UTC().Add(5 * time.Minute),
			token:          "token123",
			expectedStatus: http.StatusForbidden,
			expectedError:  "approval_request_hash_mismatch",
		},
		{
			name:             "expired",
			requestBody:      "{}",
			approvalStatus:   "APPROVED",
			expiresAt:        time.Now().UTC().Add(-1 * time.Minute),
			token:            "token123",
			expectedStatus:   http.StatusForbidden,
			expectedError:    "approval_expired",
			expectExpireExec: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, sm, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			tenantKeyHash := utils.HashString("tenant_demo_key")
			sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)).
				WithArgs(tenantKeyHash).
				WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t_demo", "Demo", true))

			payload := ToolCallRequest{
				HTTPMethod: "POST",
				Path:       "/refund",
				Query:      "",
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       tc.requestBody,
			}
			bodyHash := utils.HashBody([]byte(payload.Body))
			canonical := utils.CanonicalRequest{
				TenantID: "t_demo",
				AgentID:  "agent_demo",
				ToolID:   "mock_internal",
				Method:   payload.HTTPMethod,
				Path:     payload.Path,
				Query:    payload.Query,
				Headers:  map[string]string{"content-type": "application/json"},
				BodyHash: bodyHash,
			}
			requestHash := utils.HashCanonical(&canonical)
			storedHash := requestHash
			if tc.expectedError == "approval_request_hash_mismatch" {
				storedHash = utils.HashCanonical(&utils.CanonicalRequest{
					TenantID: "t_demo",
					AgentID:  "agent_demo",
					ToolID:   "mock_internal",
					Method:   payload.HTTPMethod,
					Path:     payload.Path,
					Query:    payload.Query,
					Headers:  map[string]string{"content-type": "application/json"},
					BodyHash: utils.HashBody([]byte("{}")),
				})
			}

			sm.ExpectQuery(regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, action_summary, risk, rule_id, request_context, reasons,
		executing_at, execution_id, failed_at, last_error_code
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
				WithArgs("t_demo", "ar_1").
				WillReturnRows(sqlmock.NewRows([]string{
					"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
					"approval_token_hash", "expires_at", "created_at", "policy_version", "action_summary", "risk", "rule_id", "request_context", "reasons",
					"executing_at", "execution_id", "failed_at", "last_error_code",
				}).AddRow(
					"ar_1", "t_demo", "agent_demo", "mock_internal", "PAYMENT.REFUND", storedHash, tc.approvalStatus,
					utils.HashString("token123"), tc.expiresAt, time.Now().UTC(), "p_v1", "Refund", "HIGH", "rule_approval", nil, nil,
					nil, nil, nil, nil,
				))

			if tc.expectExpireExec {
				sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXPIRED', decided_at = $1
		WHERE tenant_id = $2 AND approval_request_id = $3 AND status IN ('PENDING','APPROVED')`)).
					WithArgs(sqlmock.AnyArg(), "t_demo", "ar_1").
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			storeAPI := store.New(db)
			publicDeps := Dependencies{
				Store:   storeAPI,
				Metrics: newTestMetrics(),
				Config:  config.Config{BodyLimitSize: 256 * 1024, ResponseLimit: 256 * 1024},
			}

			ctx, req, rec := testhelpers.MakeRequestWithParams(
				http.MethodPost,
				testhelpers.MakeBody(payload),
				testhelpers.Params{Names: []string{"tool_id"}, Values: []string{"mock_internal"}},
			)
			req.Header.Set("X-Tenant-Key", "tenant_demo_key")
			req.Header.Set("X-Agent-Id", "agent_demo")
			req.Header.Set(approvalHeaderID, "ar_1")
			req.Header.Set(approvalHeaderToken, tc.token)

			err = publicDeps.handleToolCall(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatus, rec.Code)

			var payloadResp map[string]string
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&payloadResp))
			require.Equal(t, tc.expectedError, payloadResp["error"])

			require.NoError(t, sm.ExpectationsWereMet())
		})
	}
}
