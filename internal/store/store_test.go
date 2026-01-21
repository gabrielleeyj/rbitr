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

func TestStoreListTenants(t *testing.T) {
	cases := []struct {
		name     string
		rows     *sqlmock.Rows
		expected int
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}).
				AddRow("t1", "Tenant", "p_v1", 2),
			expected: 1,
		},
		{
			name:     "empty",
			rows:     sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}),
			expected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT t.tenant_id, t.name, tc.active_policy_version, COALESCE(tool_counts.tool_count, 0)
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN (
			SELECT tenant_id, COUNT(*) AS tool_count
			FROM rbitr.tools
			GROUP BY tenant_id
		) tool_counts ON tool_counts.tenant_id = t.tenant_id
		ORDER BY t.tenant_id`)
			mock.ExpectQuery(query).WillReturnRows(tc.rows)

			st := New(db)
			tenants, err := st.ListTenants(context.Background())
			require.NoError(t, err)
			require.Len(t, tenants, tc.expected)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetTenant(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}).
				AddRow("t1", "Tenant", "p_v1", 1),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT t.tenant_id, t.name, tc.active_policy_version, COALESCE(tool_counts.tool_count, 0)
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN (
			SELECT tenant_id, COUNT(*) AS tool_count
			FROM rbitr.tools
			GROUP BY tenant_id
		) tool_counts ON tool_counts.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetTenant(context.Background(), "t1")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreGetTenantKeyHash(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"key_hash"}).AddRow("hash"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"key_hash"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT key_hash FROM rbitr.tenant_keys WHERE tenant_id = $1`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetTenantKeyHash(context.Background(), "t1")
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

func TestStoreListTools(t *testing.T) {
	cases := []struct {
		name     string
		rows     *sqlmock.Rows
		expected int
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}).
				AddRow("tool1", "t1", "http://example", "bearer", "token"),
			expected: 1,
		},
		{
			name:     "empty",
			rows:     sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value"}),
			expected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 ORDER BY tool_id`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			tools, err := st.ListTools(context.Background(), "t1")
			require.NoError(t, err)
			require.Len(t, tools, tc.expected)
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
			rows: sqlmock.NewRows([]string{"policy_version", "tenant_id", "rego_module", "created_at"}).
				AddRow("p_v1", "t1", "module", time.Now()),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"policy_version", "tenant_id", "rego_module", "created_at"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT pv.policy_version, pv.tenant_id, pv.rego_module, pv.created_at
		FROM rbitr.tenant_config tc
		JOIN rbitr.policy_versions pv
			ON pv.tenant_id = tc.tenant_id
			AND pv.policy_version = tc.active_policy_version
		WHERE tc.tenant_id = $1`)
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

func TestStoreGetTenantConfig(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tenant_id", "active_policy_version", "created_at", "updated_at"}).
				AddRow("t1", "p_v1", time.Now(), time.Now()),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id", "active_policy_version", "created_at", "updated_at"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tenant_id, active_policy_version, created_at, updated_at FROM rbitr.tenant_config WHERE tenant_id = $1`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetTenantConfig(context.Background(), "t1")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreListPolicyVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT tenant_id, policy_version, rego_module, created_at, created_by, notes
		FROM rbitr.policy_versions
		WHERE tenant_id = $1
		ORDER BY created_at DESC`)
	mock.ExpectQuery(query).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "policy_version", "rego_module", "created_at", "created_by", "notes"}).
			AddRow("t1", "p_v1", "module", time.Now(), "admin", "notes"))

	st := New(db)
	versions, err := st.ListPolicyVersions(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetPolicyVersion(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{"tenant_id", "policy_version", "rego_module", "created_at", "created_by", "notes"}).
				AddRow("t1", "p_v1", "module", time.Now(), "admin", "notes"),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id", "policy_version", "rego_module", "created_at", "created_by", "notes"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tenant_id, policy_version, rego_module, created_at, created_by, notes
		FROM rbitr.policy_versions
		WHERE tenant_id = $1 AND policy_version = $2`)
			mock.ExpectQuery(query).WithArgs("t1", "p_v1").WillReturnRows(tc.rows)

			st := New(db)
			_, err = st.GetPolicyVersion(context.Background(), "t1", "p_v1")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreCreatePolicyVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes)
		VALUES ($1, $2, $3, $4, $5, $6)`)).
		WithArgs("t1", "p_v2", "module", sqlmock.AnyArg(), "admin", "notes").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.CreatePolicyVersion(context.Background(), "t1", "p_v2", "module", "admin", "notes"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStorePublishPolicyVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM rbitr.policy_versions WHERE tenant_id = $1 AND policy_version = $2)`)).
		WithArgs("t1", "p_v2").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET active_policy_version = $2, updated_at = $4`)).
		WithArgs("t1", "p_v2", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.PublishPolicyVersion(context.Background(), "t1", "p_v2"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRollbackPolicyVersionWithTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM rbitr.policy_versions WHERE tenant_id = $1 AND policy_version = $2)`)).
		WithArgs("t1", "p_v1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET active_policy_version = $2, updated_at = $4`)).
		WithArgs("t1", "p_v1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.RollbackPolicyVersion(context.Background(), "t1", "p_v1"))
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestStoreListRiskOverrides(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT tenant_id, action_type, action_risk, updated_at
		FROM rbitr.action_risk_overrides
		WHERE tenant_id = $1
		ORDER BY action_type`)
	mock.ExpectQuery(query).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "action_type", "action_risk", "updated_at"}).
			AddRow("t1", "DATA.EXPORT", "HIGH", time.Now()))

	st := New(db)
	overrides, err := st.ListRiskOverrides(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, overrides, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreDeleteRiskOverride(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`)).
		WithArgs("t1", "DATA.EXPORT").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.DeleteRiskOverride(context.Background(), "t1", "DATA.EXPORT"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertADR(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`)
	mock.ExpectExec(query).
		WithArgs(
			"d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "v1", "LOW", "rule", 10, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "p_v1", "reason", "hash", "resp", "ar1", sqlmock.AnyArg(),
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
		DecisionVersion:   "v1",
		DecisionRisk:      "LOW",
		Reason:            "reason",
		RuleID:            "rule",
		RulePriority:      10,
		Reasons:           []models.DecisionReason{{Code: "R1", Message: "reason"}},
		Constraints:       map[string]any{},
		Tags:              []string{"tag"},
		PolicyVersion:     "p_v1",
		RequestHash:       "hash",
		ResponseHash:      "resp",
		ApprovalRequestID: "ar1",
		CreatedAt:         time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListEvidenceFiltered(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"decision_id", "request_id", "tenant_id", "agent_id", "tool_id", "action_type", "action_risk",
		"action_summary", "decision", "decision_version", "decision_risk", "rule_id", "rule_priority",
		"reasons", "constraints", "tags", "policy_version", "reason", "request_hash",
		"response_hash", "approval_request_id", "created_at",
	}).AddRow(
		"d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "v1", "LOW", "rule", 10,
		[]byte(`[{"code":"R1","message":"ok"}]`),
		[]byte(`{}`),
		"{tag1}",
		"p_v1", "reason", "hash", "resp", "ar1", time.Now(),
	)
	mock.ExpectQuery("SELECT decision_id").
		WithArgs("t1", 1).
		WillReturnRows(rows)

	st := New(db)
	records, err := st.ListEvidenceFiltered(context.Background(), "t1", "", "", "", nil, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
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

func TestStoreListAuditEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"audit_event_id", "tenant_id", "actor_type", "actor_id", "actor_display",
		"action", "resource_type", "resource_id", "before", "after",
		"request_id", "ip", "user_agent", "created_at",
	}).AddRow(
		"ae_1", "t1", "admin_key", "admin", "admin",
		"POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", "p_v1", []byte(`{}`), []byte(`{}`),
		"req", "127.0.0.1", "agent", time.Now(),
	)
	mock.ExpectQuery("SELECT audit_event_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(rows)

	st := New(db)
	events, err := st.ListAuditEvents(context.Background(), "t1", 10, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertAuditEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`)
	mock.ExpectExec(query).
		WithArgs(
			"ae_1", "t1", "admin_key", "admin", "admin", "ACTION", "RESOURCE", "res",
			[]byte(`{}`), []byte(`{}`), "req", "127.0.0.1", "agent", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.InsertAuditEvent(context.Background(), models.AdminAuditEvent{
		AuditEventID: "ae_1",
		TenantID:     "t1",
		ActorType:    "admin_key",
		ActorID:      "admin",
		ActorDisplay: "admin",
		Action:       "ACTION",
		ResourceType: "RESOURCE",
		ResourceID:   "res",
		Before:       []byte(`{}`),
		After:        []byte(`{}`),
		RequestID:    "req",
		IP:           "127.0.0.1",
		UserAgent:    "agent",
		CreatedAt:    time.Now(),
	}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
		FROM rbitr.action_decisions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2`)
	rows := sqlmock.NewRows([]string{
		"decision_id", "request_id", "tenant_id", "agent_id", "tool_id", "action_type", "action_risk",
		"action_summary", "decision", "decision_version", "decision_risk", "rule_id", "rule_priority",
		"reasons", "constraints", "tags", "policy_version", "reason", "request_hash",
		"response_hash", "approval_request_id", "created_at",
	}).AddRow(
		"d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "v1", "LOW", "rule", 10,
		`[{"code":"R1","message":"reason"}]`, `{}`, "{tag}", "p_v1", "reason", "hash", "resp", "ar1", time.Now(),
	)

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
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, policy_version) DO UPDATE SET rego_module = $3`)).
		WithArgs("t1", "p_v2", "module", sqlmock.AnyArg(), "legacy admin policy update").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM rbitr.policy_versions WHERE tenant_id = $1 AND policy_version = $2)`)).
		WithArgs("t1", "p_v2").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET active_policy_version = $2, updated_at = $4`)).
		WithArgs("t1", "p_v2", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

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

func TestStoreGetBootstrapComplete(t *testing.T) {
	cases := []struct {
		name     string
		rows     *sqlmock.Rows
		expected bool
	}{
		{
			name:     "not set",
			rows:     sqlmock.NewRows([]string{"value"}),
			expected: false,
		},
		{
			name:     "set",
			rows:     sqlmock.NewRows([]string{"value"}).AddRow("true"),
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
				WithArgs(bootstrapKey).
				WillReturnRows(tc.rows)

			st := New(db)
			value, err := st.GetBootstrapComplete(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expected, value)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
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

func TestStoreGetAdminWriteLock(t *testing.T) {
	cases := []struct {
		name     string
		rows     *sqlmock.Rows
		expected bool
	}{
		{
			name:     "not set",
			rows:     sqlmock.NewRows([]string{"value"}),
			expected: false,
		},
		{
			name:     "set",
			rows:     sqlmock.NewRows([]string{"value"}).AddRow("true"),
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
				WithArgs(adminWriteLockKey).
				WillReturnRows(tc.rows)

			st := New(db)
			value, err := st.GetAdminWriteLock(context.Background())
			require.NoError(t, err)
			require.Equal(t, tc.expected, value)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreUpdateLocked(t *testing.T) {
	cases := []struct {
		name string
		call func(StoreAPI) error
	}{
		{
			name: "tenant config",
			call: func(st StoreAPI) error {
				return st.UpdateTenantConfig(context.Background(), "t1", "Name", "key")
			},
		},
		{
			name: "tool config",
			call: func(st StoreAPI) error {
				return st.UpdateToolConfig(context.Background(), "t1", "tool", "http://example", "bearer", "token")
			},
		},
		{
			name: "policy",
			call: func(st StoreAPI) error {
				return st.UpdatePolicy(context.Background(), "t1", "module", "p_v2")
			},
		},
		{
			name: "policy create",
			call: func(st StoreAPI) error {
				return st.CreatePolicyVersion(context.Background(), "t1", "p_v2", "module", "admin", "notes")
			},
		},
		{
			name: "policy publish",
			call: func(st StoreAPI) error {
				return st.PublishPolicyVersion(context.Background(), "t1", "p_v2")
			},
		},
		{
			name: "policy rollback",
			call: func(st StoreAPI) error {
				return st.RollbackPolicyVersion(context.Background(), "t1", "p_v2")
			},
		},
		{
			name: "risk override",
			call: func(st StoreAPI) error {
				return st.UpdateRiskOverride(context.Background(), "t1", "DATA.EXPORT", "HIGH")
			},
		},
		{
			name: "risk override delete",
			call: func(st StoreAPI) error {
				return st.DeleteRiskOverride(context.Background(), "t1", "DATA.EXPORT")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
				WithArgs(adminWriteLockKey).
				WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(settingTrue))

			st := New(db)
			err = tc.call(st)
			require.ErrorIs(t, err, ErrAdminWriteLocked)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
