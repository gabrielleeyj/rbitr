package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/gabrielleeyj/rbitr/internal/models"
)

func TestStoreGetTenantByKeyHash(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tenant_id", "name"}).AddRow("t1", "Tenant"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id", "name"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT t.tenant_id, t.name
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1`)
			mock.ExpectQuery(query).WithArgs("hash").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetTenantByKeyHash(context.Background(), "hash")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetAdminKeyByHash(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"admin_key_id", "key_hash", "scopes"}).AddRow("a1", "hash", "{admin:write}"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"admin_key_id", "key_hash", "scopes"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT admin_key_id, key_hash, scopes FROM rbitr.admin_keys WHERE key_hash = $1`)
			mock.ExpectQuery(query).WithArgs("hash").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetAdminKeyByHash(context.Background(), "hash")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetTool(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}).
				AddRow("jira", "t1", "http://example", "bearer", "token"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)
			mock.ExpectQuery(query).WithArgs("t1", "jira").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetTool(context.Background(), "t1", "jira")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetPolicy(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"policy_id", "tenant_id", "rego_module", "policy_version", "updated_at"}).
				AddRow("p1", "t1", "module", "p_v1", time.Now()),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"policy_id", "tenant_id", "rego_module", "policy_version", "updated_at"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT policy_id, tenant_id, rego_module, policy_version, updated_at FROM rbitr.policies WHERE tenant_id = $1`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetPolicy(context.Background(), "t1")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetRiskOverride(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"action_risk"}).AddRow("HIGH"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"action_risk"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT action_risk FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`)
			mock.ExpectQuery(query).WithArgs("t1", "DATA.EXPORT").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetRiskOverride(context.Background(), "t1", "DATA.EXPORT")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreInsertADR(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, reason, rule_id, policy_version, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`)
	mock.ExpectExec(query).
		WithArgs(
			"d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "reason", "rule", "p_v1", "hash", "resp", "ar1", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.InsertADR(context.Background(), models.ActionDecisionRecord{
		DecisionID:        "d1",
		RequestID:         "r1",
		TenantID:          "t1",
		AgentID:           "a1",
		ToolID:            "tool",
		ActionType:        "TYPE",
		ActionRisk:        "LOW",
		ActionSummary:     "summary",
		Decision:          "ALLOW",
		Reason:            "reason",
		RuleID:            "rule",
		PolicyVersion:     "p_v1",
		RequestHash:       "hash",
		ResponseHash:      "resp",
		ApprovalRequestID: "ar1",
		CreatedAt:         time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertApprovalRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.approval_requests (
		approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash,
		status, expires_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`)
	mock.ExpectExec(query).
		WithArgs("ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.InsertApprovalRequest(context.Background(), models.ApprovalRequest{
		ApprovalRequestID: "ar1",
		TenantID:          "t1",
		AgentID:           "a1",
		ToolID:            "tool",
		ActionType:        "TYPE",
		RequestHash:       "hash",
		Status:            "PENDING",
		ExpiresAt:         time.Now(),
		CreatedAt:         time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, reason, rule_id, policy_version, request_hash,
		response_hash, approval_request_id, created_at
		FROM rbitr.action_decisions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2`)
	rows := sqlmock.NewRows([]string{
		"decision_id", "request_id", "tenant_id", "agent_id", "tool_id", "action_type", "action_risk",
		"action_summary", "decision", "reason", "rule_id", "policy_version", "request_hash",
		"response_hash", "approval_request_id", "created_at",
	}).AddRow("d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "reason", "rule", "p_v1", "hash", "resp", "ar1", time.Now())

	mock.ExpectQuery(query).WithArgs("t1", 50).WillReturnRows(rows)

	st := New(db)
	records, err := st.ListEvidence(context.Background(), "t1", 50)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "d1", records[0].DecisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpdateTenantConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.tenants SET name = $1 WHERE tenant_id = $2`)).
		WithArgs("New Name", "t1").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.tenant_keys SET key_hash = $1 WHERE tenant_id = $2`)).
		WithArgs(sqlmock.AnyArg(), "t1").WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpdateTenantConfig(context.Background(), "t1", "New Name", "newkey")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpdateToolConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.tools SET base_url = $1, auth_type = $2, auth_value = $3 WHERE tenant_id = $4 AND tool_id = $5`)).
		WithArgs("http://example", "bearer", "token", "t1", "tool").WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpdateToolConfig(context.Background(), "t1", "tool", "http://example", "bearer", "token")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpdatePolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.policies SET rego_module = $1, policy_version = $2, updated_at = $3 WHERE tenant_id = $4`)).
		WithArgs("module", "p_v2", sqlmock.AnyArg(), "t1").WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpdatePolicy(context.Background(), "t1", "module", "p_v2")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpdateRiskOverride(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.action_risk_overrides (tenant_id, action_type, action_risk, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, action_type) DO UPDATE SET action_risk = $3, updated_at = $4`)).
		WithArgs("t1", "DATA.EXPORT", "HIGH", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpdateRiskOverride(context.Background(), "t1", "DATA.EXPORT", "HIGH")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkBootstrapComplete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`)
	mock.ExpectExec(query).
		WithArgs(bootstrapKey, "true", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.MarkBootstrapComplete(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreSetAdminWriteLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`)
	mock.ExpectExec(query).
		WithArgs(adminWriteLockKey, "true", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.SetAdminWriteLock(context.Background(), true))
	require.NoError(t, mock.ExpectationsWereMet())
}
