package public

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/testhelpers"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

func TestHandleToolCallApprovedExecutionIntegration(t *testing.T) {
	db, sm, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	tenantKeyHash := utils.HashString("tenant_demo_key")
	sm.ExpectQuery(regexp.QuoteMeta(`SELECT t.tenant_id, t.name
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1`)).
		WithArgs(tenantKeyHash).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "name"}).AddRow("t_demo", "Demo"))

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
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, reasons
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)).
		WithArgs("t_demo", "ar_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
			"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by", "decision_comment",
			"executed_at", "executed_request_id", "executed_decision_id", "request_decision_id", "action_summary",
			"risk", "rule_id", "reasons",
		}).AddRow(
			"ar_1", "t_demo", "agent_demo", "mock_internal", "PAYMENT.REFUND", requestHash, "APPROVED",
			utils.HashString("token123"), time.Now().UTC().Add(5*time.Minute), time.Now().UTC(), "p_v1", nil, nil, nil,
			nil, nil, nil, "d_req", "Refund", "MEDIUM", "rule_approval", nil,
		))

	sm.ExpectQuery(regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)).
		WithArgs("t_demo", "mock_internal").
		WillReturnRows(sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}).
			AddRow("mock_internal", "t_demo", "http://mock.local", "", ""))

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

	sm.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.approval_requests
		SET status = 'EXECUTED', executed_at = $1, executed_request_id = $2, executed_decision_id = $3
		WHERE tenant_id = $4 AND approval_request_id = $5 AND status = 'APPROVED'`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t_demo", "ar_1").
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
