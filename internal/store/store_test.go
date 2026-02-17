package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strconv"
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
			rows: sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}).AddRow("t1", "Tenant", true),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id", "name", "enabled"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true`)
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

func TestStoreSetDefaultApprovalTTLSeconds(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`)
	mock.ExpectExec(query).
		WithArgs("default_approval_ttl_seconds", "900", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.SetDefaultApprovalTTLSeconds(context.Background(), 900))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetDefaultApprovalTTLSeconds(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs("default_approval_ttl_seconds").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("900"))

	st := New(db)
	value, err := st.GetDefaultApprovalTTLSeconds(context.Background())
	require.NoError(t, err)
	require.Equal(t, 900, value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetDefaultApprovalTTLSecondsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs("default_approval_ttl_seconds").
		WillReturnError(sql.ErrNoRows)

	st := New(db)
	_, err = st.GetDefaultApprovalTTLSeconds(context.Background())
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetDefaultApprovalTTLSecondsInvalid(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs("default_approval_ttl_seconds").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("bad"))

	st := New(db)
	_, err = st.GetDefaultApprovalTTLSeconds(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
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
			rows: sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}).
				AddRow("jira", "t1", "http://example", "bearer", "token", "http_url", nil, nil, nil),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`)
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
			rows: sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}).
				AddRow("tool1", "t1", "http://example", "bearer", "token", "http_url", nil, nil, nil),
			expected: 1,
		},
		{
			name:     "empty",
			rows:     sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}),
			expected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 ORDER BY tool_id`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			tools, err := st.ListTools(context.Background(), "t1")
			require.NoError(t, err)
			require.Len(t, tools, tc.expected)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreListToolsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 ORDER BY tool_id`)
	mock.ExpectQuery(query).WithArgs("t1").WillReturnError(errors.New("query failed"))

	st := New(db)
	_, err = st.ListTools(context.Background(), "t1")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListToolsRowError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json FROM rbitr.tools WHERE tenant_id = $1 ORDER BY tool_id`)
	rows := sqlmock.NewRows([]string{"tool_id", "tenant_id", "base_url", "auth_type", "auth_value", "transport", "mcp_upstream_url", "description", "input_schema_json"}).
		AddRow("tool1", "t1", "http://example", "bearer", "token", "http_url", nil, nil, nil).
		RowError(0, errors.New("row error"))
	mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(rows)

	st := New(db)
	_, err = st.ListTools(context.Background(), "t1")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListTenantsError(t *testing.T) {
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
	mock.ExpectQuery(query).WillReturnError(errors.New("query failed"))

	st := New(db)
	_, err = st.ListTenants(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListTenantsRowError(t *testing.T) {
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
	rows := sqlmock.NewRows([]string{"tenant_id", "name", "active_policy_version", "tool_count"}).
		AddRow("t1", "Tenant", "p_v1", 1).
		RowError(0, errors.New("row error"))
	mock.ExpectQuery(query).WillReturnRows(rows)

	st := New(db)
	_, err = st.ListTenants(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
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

func TestStorePublishPolicyVersionNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM rbitr.policy_versions WHERE tenant_id = $1 AND policy_version = $2)`)).
		WithArgs("t1", "p_v2").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	st := New(db)
	err = st.PublishPolicyVersion(context.Background(), "t1", "p_v2")
	require.ErrorIs(t, err, ErrNotFound)
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

func TestStoreRollbackPolicyVersionMissingActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT active_policy_version FROM rbitr.tenant_config WHERE tenant_id = $1`)).
		WithArgs("t1").
		WillReturnError(sql.ErrNoRows)

	st := New(db)
	err = st.RollbackPolicyVersion(context.Background(), "t1", "")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRollbackPolicyVersionMissingPrevious(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT active_policy_version FROM rbitr.tenant_config WHERE tenant_id = $1`)).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"active_policy_version"}).AddRow("p_v2"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pv.policy_version
		FROM rbitr.policy_versions pv
		WHERE pv.tenant_id = $1
			AND pv.policy_version <> $2
		ORDER BY pv.created_at DESC
		LIMIT 1`)).
		WithArgs("t1", "p_v2").
		WillReturnError(sql.ErrNoRows)

	st := New(db)
	err = st.RollbackPolicyVersion(context.Background(), "t1", "")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRollbackPolicyVersionSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT active_policy_version FROM rbitr.tenant_config WHERE tenant_id = $1`)).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"active_policy_version"}).AddRow("p_v2"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pv.policy_version
		FROM rbitr.policy_versions pv
		WHERE pv.tenant_id = $1
			AND pv.policy_version <> $2
		ORDER BY pv.created_at DESC
		LIMIT 1`)).
		WithArgs("t1", "p_v2").
		WillReturnRows(sqlmock.NewRows([]string{"policy_version"}).AddRow("p_v1"))
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
	err = st.RollbackPolicyVersion(context.Background(), "t1", "")
	require.NoError(t, err)
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

func TestStoreListRiskOverridesError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT tenant_id, action_type, action_risk, updated_at
		FROM rbitr.action_risk_overrides
		WHERE tenant_id = $1
		ORDER BY action_type`)
	mock.ExpectQuery(query).
		WithArgs("t1").
		WillReturnError(errors.New("query failed"))

	st := New(db)
	_, err = st.ListRiskOverrides(context.Background(), "t1")
	require.Error(t, err)
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

func TestStoreDeleteRiskOverrideLocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(settingTrue))

	st := New(db)
	err = st.DeleteRiskOverride(context.Background(), "t1", "DATA.EXPORT")
	require.ErrorIs(t, err, ErrAdminWriteLocked)
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

func TestStoreInsertADRError(t *testing.T) {
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
		WillReturnError(errors.New("insert failed"))

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
	require.Error(t, err)
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

func TestStoreListEvidenceFilteredInvalidJSON(t *testing.T) {
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
		`[]`, `{bad`, "{tag}", "p_v1", "reason", "hash", "resp", "ar1", time.Now(),
	)
	mock.ExpectQuery("SELECT decision_id").
		WithArgs("t1", 1).
		WillReturnRows(rows)

	st := New(db)
	_, err = st.ListEvidenceFiltered(context.Background(), "t1", "", "", "", nil, 1)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListEvidenceFilteredWithSince(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().Add(-time.Hour).UTC()
	rows := sqlmock.NewRows([]string{
		"decision_id", "request_id", "tenant_id", "agent_id", "tool_id", "action_type", "action_risk",
		"action_summary", "decision", "decision_version", "decision_risk", "rule_id", "rule_priority",
		"reasons", "constraints", "tags", "policy_version", "reason", "request_hash",
		"response_hash", "approval_request_id", "created_at",
	}).AddRow(
		"d1", "r1", "t1", "a1", "tool", "TYPE", "LOW", "summary", "ALLOW", "v1", "LOW", "rule", 10,
		[]byte(`[]`),
		[]byte(`{}`),
		"{tag1}",
		"p_v1", "reason", "hash", "resp", "ar1", time.Now(),
	)
	mock.ExpectQuery("SELECT decision_id").
		WithArgs("t1", since, 1).
		WillReturnRows(rows)

	st := New(db)
	records, err := st.ListEvidenceFiltered(context.Background(), "t1", "", "", "", &since, 1)
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
		status, approval_token_hash, expires_at, created_at, policy_version,
		decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id,
		request_decision_id, action_summary, risk, rule_id, request_context, reasons
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`)
	mock.ExpectExec(query).
		WithArgs(
			"ar1",
			"t1",
			"a1",
			"tool",
			"TYPE",
			"hash",
			"PENDING",
			"",
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

func TestStoreInsertApprovalRequestBadReasons(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

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
		Reasons:           []models.DecisionReason{{Code: "bad", Message: string([]byte{0xff})}},
	})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListApprovalRequests(t *testing.T) {
	cases := []struct {
		name   string
		status string
		args   []any
		query  string
	}{
		{
			name:   "no status default limit",
			status: "",
			args:   []any{"t1", 50, 0},
			query:  "tenant_id = $1",
		},
		{
			name:   "status filter",
			status: "PENDING",
			args:   []any{"t1", "PENDING", 10, 0},
			query:  "tenant_id = $1 AND status = $2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE ` + tc.query + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(len(tc.args)-1) + ` OFFSET $` + strconv.Itoa(len(tc.args)))
			rows := sqlmock.NewRows([]string{
				"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
				"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by",
				"decision_comment", "executed_at", "executed_request_id", "executed_decision_id", "request_decision_id",
				"action_summary", "risk", "rule_id", "request_context", "reasons",
			}).AddRow(
				"ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING",
				"token", time.Now(), time.Now(), "p_v1", nil, nil,
				nil, nil, nil, nil, nil,
				nil, nil, nil, []byte(`{"path":"/refund"}`), []byte(`[]`),
			)
			mock.ExpectQuery(query).WithArgs(toDriverValues(tc.args)...).WillReturnRows(rows)

			st := New(db)
			limit := 0
			offset := -1
			if tc.status != "" {
				limit = 10
				offset = 0
			}
			results, err := st.ListApprovalRequests(context.Background(), "t1", tc.status, limit, offset)
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Equal(t, "/refund", results[0].RequestContext["path"])
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func toDriverValues(args []any) []driver.Value {
	out := make([]driver.Value, 0, len(args))
	for _, arg := range args {
		out = append(out, arg)
	}
	return out
}

func TestStoreGetApprovalRequestNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`)
	mock.ExpectQuery(query).WithArgs("t1", "ar1").WillReturnError(sql.ErrNoRows)

	st := New(db)
	_, err = st.GetApprovalRequest(context.Background(), "t1", "ar1")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkApprovalExpiredInvalidState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status FROM rbitr.approval_requests").
		WithArgs("t1", "ar1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("DENIED"))

	st := New(db)
	err = st.MarkApprovalExpired(context.Background(), "t1", "ar1", time.Now().UTC())
	require.ErrorIs(t, err, ErrInvalidState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScanApprovalRequestNulls(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons`)
	rows := sqlmock.NewRows([]string{
		"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
		"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by",
		"decision_comment", "executed_at", "executed_request_id", "executed_decision_id", "request_decision_id",
		"action_summary", "risk", "rule_id", "request_context", "reasons",
	}).AddRow(
		"ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING",
		"token", time.Now(), time.Now(), nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, []byte(`[]`),
	)
	mock.ExpectQuery(query).WillReturnRows(rows)

	row := db.QueryRowContext(context.Background(), query)
	approval, err := scanApprovalRequest(row)
	require.NoError(t, err)
	require.Equal(t, "ar1", approval.ApprovalRequestID)
	require.Empty(t, approval.DecidedBy)
	require.Nil(t, approval.DecidedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScanApprovalRequestInvalidReasons(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons`)
	rows := sqlmock.NewRows([]string{
		"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
		"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by",
		"decision_comment", "executed_at", "executed_request_id", "executed_decision_id", "request_decision_id",
		"action_summary", "risk", "rule_id", "request_context", "reasons",
	}).AddRow(
		"ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING",
		"token", time.Now(), time.Now(), nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, []byte(`{bad`),
	)
	mock.ExpectQuery(query).WillReturnRows(rows)

	row := db.QueryRowContext(context.Background(), query)
	_, err = scanApprovalRequest(row)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestStoreApproveApprovalRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs("APPROVED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.ApproveApprovalRequest(context.Background(), "t1", "ar1", "admin", "ok", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreApproveApprovalRequestInvalidState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs("APPROVED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status FROM rbitr.approval_requests").
		WithArgs("t1", "ar1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("DENIED"))

	st := New(db)
	err = st.ApproveApprovalRequest(context.Background(), "t1", "ar1", "admin", "ok", time.Now().UTC())
	require.ErrorIs(t, err, ErrInvalidState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreDenyApprovalRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs("DENIED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.DenyApprovalRequest(context.Background(), "t1", "ar1", "admin", "no", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRevokeApprovalRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs("REVOKED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.RevokeApprovalRequest(context.Background(), "t1", "ar1", "admin", "revoke", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkApprovalExecuted(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), "req1", "dec1", "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.MarkApprovalExecuted(context.Background(), "t1", "ar1", "req1", "dec1", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkApprovalExecutedInvalidState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), "req1", "dec1", "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status FROM rbitr.approval_requests").
		WithArgs("t1", "ar1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("DENIED"))

	st := New(db)
	err = st.MarkApprovalExecuted(context.Background(), "t1", "ar1", "req1", "dec1", time.Now().UTC())
	require.ErrorIs(t, err, ErrInvalidState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreClaimApprovalExecution(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), "t1", "ar1", sqlmock.AnyArg(), "token_hash", "request_hash").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.ClaimApprovalExecution(context.Background(), "t1", "ar1", "token_hash", "request_hash", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreClaimApprovalExecutionInvalidState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), "t1", "ar1", sqlmock.AnyArg(), "token_hash", "request_hash").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT status FROM rbitr.approval_requests").
		WithArgs("t1", "ar1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("EXECUTING"))

	st := New(db)
	err = st.ClaimApprovalExecution(context.Background(), "t1", "ar1", "token_hash", "request_hash", time.Now().UTC())
	require.ErrorIs(t, err, ErrInvalidState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkApprovalExecutionFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec("UPDATE rbitr.approval_requests").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "t1", "ar1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.MarkApprovalExecutionFailed(context.Background(), "t1", "ar1", "UPSTREAM_ERROR", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListAuditEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"audit_event_id", "tenant_id", "stream_id", "event_hash", "prev_hash",
		"actor_type", "actor_id", "actor_display", "action", "resource_type", "resource_id",
		"before", "after", "request_id", "ip", "user_agent", "created_at",
	}).AddRow(
		"ae_1", "t1", "t1", "hash", nil,
		"admin_key", "admin", "admin", "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", "p_v1",
		[]byte(`{}`), []byte(`{}`), "req", "127.0.0.1", "agent", time.Now(),
	)
	mock.ExpectQuery("SELECT audit_event_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(rows)

	st := New(db)
	events, err := st.ListAuditEvents(context.Background(), "t1", 10, 0, "", "", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListAuditEventsFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"audit_event_id", "tenant_id", "stream_id", "event_hash", "prev_hash",
		"actor_type", "actor_id", "actor_display", "action", "resource_type", "resource_id",
		"before", "after", "request_id", "ip", "user_agent", "created_at",
	}).AddRow(
		"ae_1", nil, "global", "hash", nil,
		"admin_key", nil, nil, "POLICY.VERSION.CREATE", "POLICY", nil,
		nil, nil, nil, nil, nil, time.Now(),
	)
	mock.ExpectQuery("SELECT audit_event_id").
		WithArgs("POLICY.VERSION.CREATE", "POLICY", "admin", 10, 0).
		WillReturnRows(rows)

	st := New(db)
	events, err := st.ListAuditEvents(context.Background(), "", 10, 0, "POLICY.VERSION.CREATE", "POLICY", "admin", nil, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "", events[0].TenantID)
	require.Equal(t, "", events[0].ActorID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListAuditEventsExport(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"audit_event_id", "tenant_id", "stream_id", "event_hash", "prev_hash",
		"actor_type", "actor_id", "actor_display", "action", "resource_type", "resource_id",
		"before", "after", "request_id", "ip", "user_agent", "created_at",
	}).AddRow(
		"ae_1", "t1", "t1", "hash", "prev",
		"admin_key", "admin", "admin", "ACTION", "RESOURCE", "res",
		nil, nil, nil, nil, nil, time.Now(),
	)
	mock.ExpectQuery("SELECT audit_event_id").
		WithArgs("t1", 10, 0).
		WillReturnRows(rows)

	st := New(db)
	events, err := st.ListAuditEventsExport(context.Background(), "t1", 10, 0, "", "", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "t1", events[0].StreamID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreDeleteAuditEventsBefore(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM rbitr.admin_audit_events WHERE created_at < $1`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	st := New(db)
	rows, err := st.DeleteAuditEventsBefore(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreInsertAuditEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT event_hash
		FROM rbitr.admin_audit_events
		WHERE stream_id = $1 AND event_hash IS NOT NULL
		ORDER BY created_at DESC, audit_event_id DESC
		LIMIT 1`)).
		WithArgs("t1").
		WillReturnError(sql.ErrNoRows)

	query := regexp.QuoteMeta(`INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`)
	mock.ExpectExec(query).
		WithArgs(
			"ae_1", "t1", "t1", sqlmock.AnyArg(), sqlmock.AnyArg(),
			"admin_key", "admin", "admin", "ACTION", "RESOURCE", "res",
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

func TestStoreInsertAuditEventChainsHashes(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT event_hash
		FROM rbitr.admin_audit_events
		WHERE stream_id = $1 AND event_hash IS NOT NULL
		ORDER BY created_at DESC, audit_event_id DESC
		LIMIT 1`)).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"event_hash"}).AddRow("prevhash"))

	query := regexp.QuoteMeta(`INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`)
	mock.ExpectExec(query).
		WithArgs(
			"ae_2", "t1", "t1", sqlmock.AnyArg(), "prevhash",
			"admin_key", "admin", "admin", "ACTION", "RESOURCE", "res",
			[]byte(`{}`), []byte(`{}`), "req", "127.0.0.1", "agent", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	require.NoError(t, st.InsertAuditEvent(context.Background(), models.AdminAuditEvent{
		AuditEventID: "ae_2",
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

func TestStoreListEvidenceInvalidJSON(t *testing.T) {
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
		`{bad`, `{}`, "{tag}", "p_v1", "reason", "hash", "resp", "ar1", time.Now(),
	)
	mock.ExpectQuery(query).WithArgs("t1", 50).WillReturnRows(rows)

	st := New(db)
	_, err = st.ListEvidence(context.Background(), "t1", 50)
	require.Error(t, err)
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

func TestStoreUpdateTenantConfigNoKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(adminWriteLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.tenants SET name = $1 WHERE tenant_id = $2`)).
		WithArgs("New Name", "t1").WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpdateTenantConfig(context.Background(), "t1", "New Name", "")
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

func TestEnsureAdminWritesAllowed(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		queryErr  error
		expectErr error
	}{
		{
			name:     "not set",
			queryErr: sql.ErrNoRows,
		},
		{
			name:      "locked",
			rows:      sqlmock.NewRows([]string{"value"}).AddRow(settingTrue),
			expectErr: ErrAdminWriteLocked,
		},
		{
			name: "unlocked",
			rows: sqlmock.NewRows([]string{"value"}).AddRow(settingFalse),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)
			if tc.queryErr != nil {
				mock.ExpectQuery(query).WithArgs(adminWriteLockKey).WillReturnError(tc.queryErr)
			} else {
				mock.ExpectQuery(query).WithArgs(adminWriteLockKey).WillReturnRows(tc.rows)
			}

			st := New(db).(*Store)
			err = st.ensureAdminWritesAllowed(context.Background())
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestApprovalStateError(t *testing.T) {
	cases := []struct {
		name      string
		rowErr    error
		status    string
		expectErr error
	}{
		{
			name:      "not found",
			rowErr:    sql.ErrNoRows,
			expectErr: ErrNotFound,
		},
		{
			name:      "invalid state",
			status:    "APPROVED",
			expectErr: ErrInvalidState,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT status FROM rbitr.approval_requests WHERE tenant_id = $1 AND approval_request_id = $2`)
			if tc.rowErr != nil {
				mock.ExpectQuery(query).WithArgs("t1", "ar1").WillReturnError(tc.rowErr)
			} else {
				mock.ExpectQuery(query).WithArgs("t1", "ar1").
					WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(tc.status))
			}

			st := New(db).(*Store)
			err = st.approvalStateError(context.Background(), "t1", "ar1")
			require.ErrorIs(t, err, tc.expectErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestNullableString(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{value: "", valid: false},
		{value: "ok", valid: true},
	}

	for _, tc := range cases {
		got := nullableString(tc.value)
		if got.Valid != tc.valid {
			t.Fatalf("expected valid=%v got %v", tc.valid, got.Valid)
		}
		if got.String != tc.value {
			t.Fatalf("expected string=%q got %q", tc.value, got.String)
		}
	}
}

func TestHashKey(t *testing.T) {
	hash := hashKey("abc")
	if hash == "" || hash == "abc" {
		t.Fatalf("expected hashed key, got %q", hash)
	}
}

func TestStoreGetNotificationConfig(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		expectErr error
	}{
		{
			name: "found",
			rows: sqlmock.NewRows([]string{
				"tenant_id", "slack_webhook_enabled", "slack_webhook_secret_ref", "slack_webhook_default_channel",
				"slack_bot_enabled", "slack_bot_secret_ref", "slack_bot_default_channel", "slack_bot_signing_secret_ref",
				"email_enabled", "email_provider", "email_secret_ref", "email_from", "email_region", "email_domain", "email_default_mailing_list_id",
				"notify_approval_expiring", "notify_token_abuse", "notify_policy_invalid", "created_at", "updated_at",
			}).AddRow(
				"t1", true, "env://SLACK", "C01",
				false, nil, nil, nil,
				false, nil, nil, nil, nil, nil, nil,
				true, true, true, time.Now(), time.Now(),
			),
		},
		{
			name:      "not found",
			rows:      sqlmock.NewRows([]string{"tenant_id"}),
			expectErr: ErrNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT tenant_id, slack_webhook_enabled, slack_webhook_secret_ref, slack_webhook_default_channel,
		slack_bot_enabled, slack_bot_secret_ref, slack_bot_default_channel, slack_bot_signing_secret_ref,
		email_enabled, email_provider, email_secret_ref, email_from, email_region, email_domain, email_default_mailing_list_id,
		notify_approval_expiring, notify_token_abuse, notify_policy_invalid, created_at, updated_at
		FROM rbitr.notification_config WHERE tenant_id = $1`)
			mock.ExpectQuery(query).WithArgs("t1").WillReturnRows(tc.rows)

			st := New(db)
			config, err := st.GetNotificationConfig(context.Background(), "t1")
			if tc.expectErr != nil {
				require.ErrorIs(t, err, tc.expectErr)
				require.NoError(t, mock.ExpectationsWereMet())
				return
			}
			require.NoError(t, err)
			require.Equal(t, "t1", config.TenantID)
			require.True(t, config.SlackWebhookEnabled)
			require.Equal(t, "env://SLACK", config.SlackWebhookSecretRef)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreUpsertNotificationConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.notification_config (
		tenant_id, slack_webhook_enabled, slack_webhook_secret_ref, slack_webhook_default_channel,
		slack_bot_enabled, slack_bot_secret_ref, slack_bot_default_channel, slack_bot_signing_secret_ref,
		email_enabled, email_provider, email_secret_ref, email_from, email_region, email_domain, email_default_mailing_list_id,
		notify_approval_expiring, notify_token_abuse, notify_policy_invalid, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	ON CONFLICT (tenant_id) DO UPDATE SET
		slack_webhook_enabled = EXCLUDED.slack_webhook_enabled,
		slack_webhook_secret_ref = EXCLUDED.slack_webhook_secret_ref,
		slack_webhook_default_channel = EXCLUDED.slack_webhook_default_channel,
		slack_bot_enabled = EXCLUDED.slack_bot_enabled,
		slack_bot_secret_ref = EXCLUDED.slack_bot_secret_ref,
		slack_bot_default_channel = EXCLUDED.slack_bot_default_channel,
		slack_bot_signing_secret_ref = EXCLUDED.slack_bot_signing_secret_ref,
		email_enabled = EXCLUDED.email_enabled,
		email_provider = EXCLUDED.email_provider,
		email_secret_ref = EXCLUDED.email_secret_ref,
		email_from = EXCLUDED.email_from,
		email_region = EXCLUDED.email_region,
		email_domain = EXCLUDED.email_domain,
		email_default_mailing_list_id = EXCLUDED.email_default_mailing_list_id,
		notify_approval_expiring = EXCLUDED.notify_approval_expiring,
		notify_token_abuse = EXCLUDED.notify_token_abuse,
		notify_policy_invalid = EXCLUDED.notify_policy_invalid,
		updated_at = EXCLUDED.updated_at`)
	mock.ExpectExec(query).
		WithArgs(
			"t1",
			true,
			sql.NullString{String: "env://SLACK", Valid: true},
			sql.NullString{String: "C01", Valid: true},
			false,
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			false,
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			sql.NullString{String: "", Valid: false},
			true,
			true,
			true,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpsertNotificationConfig(context.Background(), models.NotificationConfig{
		TenantID:                   "t1",
		SlackWebhookEnabled:        true,
		SlackWebhookSecretRef:      "env://SLACK",
		SlackWebhookDefaultChannel: "C01",
		NotifyApprovalExpiring:     true,
		NotifyTokenAbuse:           true,
		NotifyPolicyInvalid:        true,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListMailingLists(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT mailing_list_id, tenant_id, name, description, created_at, updated_at
		FROM rbitr.mailing_lists
		WHERE tenant_id = $1
		ORDER BY created_at DESC`)
	mock.ExpectQuery(query).
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"mailing_list_id", "tenant_id", "name", "description", "created_at", "updated_at"}).
			AddRow("ml1", "t1", "Security", "desc", time.Now(), time.Now()))

	st := New(db)
	lists, err := st.ListMailingLists(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, lists, 1)
	require.Equal(t, "Security", lists[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetMailingListNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT mailing_list_id, tenant_id, name, description, created_at, updated_at
		FROM rbitr.mailing_lists
		WHERE tenant_id = $1 AND mailing_list_id = $2`)
	mock.ExpectQuery(query).
		WithArgs("t1", "ml1").
		WillReturnError(sql.ErrNoRows)

	st := New(db)
	_, err = st.GetMailingList(context.Background(), "t1", "ml1")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListMailingListMembers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT mailing_list_id, email, created_at
		FROM rbitr.mailing_list_members
		WHERE mailing_list_id = $1
		ORDER BY email`)
	mock.ExpectQuery(query).
		WithArgs("ml1").
		WillReturnRows(sqlmock.NewRows([]string{"mailing_list_id", "email", "created_at"}).
			AddRow("ml1", "a@example.com", time.Now()))

	st := New(db)
	members, err := st.ListMailingListMembers(context.Background(), "ml1")
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, "a@example.com", members[0].Email)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreCreateMailingList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.mailing_lists (mailing_list_id, tenant_id, name, description, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)`)).
		WithArgs("ml1", "t1", "Security", sql.NullString{String: "desc", Valid: true}, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.mailing_list_members (mailing_list_id, email, created_at)
			VALUES ($1,$2,$3)`)).
		WithArgs("ml1", "a@example.com", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	st := New(db)
	err = st.CreateMailingList(context.Background(), models.MailingList{
		MailingListID: "ml1",
		TenantID:      "t1",
		Name:          "Security",
		Description:   "desc",
	}, []string{"a@example.com"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpdateMailingList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rbitr.mailing_lists
		SET name = $1, description = $2, updated_at = $3
		WHERE tenant_id = $4 AND mailing_list_id = $5`)).
		WithArgs("Security", sql.NullString{String: "", Valid: false}, sqlmock.AnyArg(), "t1", "ml1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM rbitr.mailing_list_members WHERE mailing_list_id = $1`)).
		WithArgs("ml1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.mailing_list_members (mailing_list_id, email, created_at)
			VALUES ($1,$2,$3)`)).
		WithArgs("ml1", "b@example.com", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	st := New(db)
	err = st.UpdateMailingList(context.Background(), models.MailingList{
		MailingListID: "ml1",
		TenantID:      "t1",
		Name:          "Security",
	}, []string{"b@example.com"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreDeleteMailingList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM rbitr.mailing_lists WHERE tenant_id = $1 AND mailing_list_id = $2`)).
		WithArgs("t1", "ml1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.DeleteMailingList(context.Background(), "t1", "ml1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetNotificationSuppression(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"dedup_key", "tenant_id", "channel", "event_type", "resource_id", "severity",
		"first_seen_at", "last_seen_at", "last_sent_at", "suppressed_until", "suppressed_count", "last_payload_hash", "updated_at",
	}).AddRow(
		"d1", "t1", "slack", "APPROVAL.EXPIRING", "ar1", "WARN",
		time.Now(), time.Now(), time.Now(), time.Now(), 2, "hash", time.Now(),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until, suppressed_count, last_payload_hash, updated_at
		FROM rbitr.notification_suppressions WHERE dedup_key = $1`)).
		WithArgs("d1").
		WillReturnRows(rows)

	st := New(db)
	item, err := st.GetNotificationSuppression(context.Background(), "d1")
	require.NoError(t, err)
	require.Equal(t, "d1", item.DedupKey)
	require.Equal(t, "ar1", item.ResourceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetNotificationSuppressionNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until, suppressed_count, last_payload_hash, updated_at
		FROM rbitr.notification_suppressions WHERE dedup_key = $1`)).
		WithArgs("d1").
		WillReturnError(sql.ErrNoRows)

	st := New(db)
	_, err = st.GetNotificationSuppression(context.Background(), "d1")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreUpsertNotificationSuppression(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`INSERT INTO rbitr.notification_suppressions (
		dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until,
		suppressed_count, last_payload_hash, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (dedup_key) DO UPDATE SET
		tenant_id = EXCLUDED.tenant_id,
		channel = EXCLUDED.channel,
		event_type = EXCLUDED.event_type,
		resource_id = EXCLUDED.resource_id,
		severity = EXCLUDED.severity,
		last_seen_at = EXCLUDED.last_seen_at,
		last_sent_at = EXCLUDED.last_sent_at,
		suppressed_until = EXCLUDED.suppressed_until,
		suppressed_count = EXCLUDED.suppressed_count,
		last_payload_hash = EXCLUDED.last_payload_hash,
		updated_at = EXCLUDED.updated_at`)
	mock.ExpectExec(query).
		WithArgs(
			"d1",
			"t1",
			"slack",
			"APPROVAL.EXPIRING",
			sql.NullString{String: "ar1", Valid: true},
			"WARN",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			int64(2),
			sql.NullString{String: "hash", Valid: true},
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.UpsertNotificationSuppression(context.Background(), models.NotificationSuppression{
		DedupKey:        "d1",
		TenantID:        "t1",
		Channel:         "slack",
		EventType:       "APPROVAL.EXPIRING",
		ResourceID:      "ar1",
		Severity:        "WARN",
		FirstSeenAt:     time.Now(),
		LastSeenAt:      time.Now(),
		LastSentAt:      ptrTime(time.Now()),
		SuppressedUntil: ptrTime(time.Now()),
		SuppressedCount: 2,
		LastPayloadHash: "hash",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListNotificationSuppressions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	query := regexp.QuoteMeta(`SELECT dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until, suppressed_count, last_payload_hash, updated_at
		FROM rbitr.notification_suppressions
		WHERE tenant_id = $1
		ORDER BY last_seen_at DESC
		LIMIT $2 OFFSET $3`)
	rows := sqlmock.NewRows([]string{
		"dedup_key", "tenant_id", "channel", "event_type", "resource_id", "severity",
		"first_seen_at", "last_seen_at", "last_sent_at", "suppressed_until", "suppressed_count", "last_payload_hash", "updated_at",
	}).AddRow(
		"dk1", "t1", "slack_webhook", "APPROVAL.EXPIRING", "ar1", "WARN",
		time.Now(), time.Now(), nil, nil, 2, "hash", time.Now(),
	)
	mock.ExpectQuery(query).WithArgs("t1", 10, 0).WillReturnRows(rows)

	st := New(db)
	items, err := st.ListNotificationSuppressions(context.Background(), "t1", 10, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "dk1", items[0].DedupKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestStoreListApprovalsExpiring(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	cutoff := now.Add(5 * time.Minute)
	query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE status IN ('PENDING','APPROVED')
			AND expires_at > $1
			AND expires_at <= $2
		ORDER BY expires_at ASC`)
	rows := sqlmock.NewRows([]string{
		"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
		"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by",
		"decision_comment", "executed_at", "executed_request_id", "executed_decision_id", "request_decision_id",
		"action_summary", "risk", "rule_id", "request_context", "reasons",
	}).AddRow(
		"ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING",
		"token", now.Add(2*time.Minute), now, "p_v1", nil, nil,
		nil, nil, nil, nil, nil, "Summary", "HIGH", "rule", []byte(`{"http_method":"POST"}`), []byte(`[]`),
	)
	mock.ExpectQuery(query).WithArgs(now, cutoff).WillReturnRows(rows)

	st := New(db)
	items, err := st.ListApprovalsExpiring(context.Background(), now, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListApprovalsExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	query := regexp.QuoteMeta(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
		FROM rbitr.approval_requests
		WHERE status IN ('PENDING','APPROVED')
			AND expires_at <= $1
		ORDER BY expires_at ASC`)
	rows := sqlmock.NewRows([]string{
		"approval_request_id", "tenant_id", "agent_id", "tool_id", "action_type", "request_hash", "status",
		"approval_token_hash", "expires_at", "created_at", "policy_version", "decided_at", "decided_by",
		"decision_comment", "executed_at", "executed_request_id", "executed_decision_id", "request_decision_id",
		"action_summary", "risk", "rule_id", "request_context", "reasons",
	}).AddRow(
		"ar1", "t1", "a1", "tool", "TYPE", "hash", "PENDING",
		"token", now.Add(-time.Minute), now, "p_v1", nil, nil,
		nil, nil, nil, nil, nil, "Summary", "HIGH", "rule", []byte(`{"http_method":"POST"}`), []byte(`[]`),
	)
	mock.ExpectQuery(query).WithArgs(now).WillReturnRows(rows)

	st := New(db)
	items, err := st.ListApprovalsExpired(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreTryAdvisoryLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1)`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	st := New(db)
	ok, err := st.TryAdvisoryLock(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreReleaseAdvisoryLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1)`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true))

	st := New(db)
	err = st.ReleaseAdvisoryLock(context.Background(), 42)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetEffectiveRateLimitConfig(t *testing.T) {
	cases := []struct {
		name     string
		row      []any
		expected models.RateLimitConfig
	}{
		{
			name: "tenant override",
			row:  []any{int64(30), int64(9000), "tenant_tool", "60", "10000", "tenant_agent_tool"},
			expected: models.RateLimitConfig{
				PerMinute: 30,
				PerDay:    9000,
				Scope:     "tenant_tool",
			},
		},
		{
			name: "system fallback",
			row:  []any{nil, nil, nil, "75", "12000", "tenant_agent"},
			expected: models.RateLimitConfig{
				PerMinute: 75,
				PerDay:    12000,
				Scope:     "tenant_agent",
			},
		},
		{
			name: "hard default fallback",
			row:  []any{nil, nil, nil, nil, nil, nil},
			expected: models.RateLimitConfig{
				PerMinute: 60,
				PerDay:    10000,
				Scope:     "tenant_agent_tool",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			query := regexp.QuoteMeta(`SELECT
			tc.default_rate_limit_per_minute,
			tc.default_rate_limit_per_day,
			tc.default_rate_limit_scope,
			s_min.value,
			s_day.value,
			s_scope.value
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN rbitr.system_settings s_min ON s_min.key = $2
		LEFT JOIN rbitr.system_settings s_day ON s_day.key = $3
		LEFT JOIN rbitr.system_settings s_scope ON s_scope.key = $4
		WHERE t.tenant_id = $1`)
			mock.ExpectQuery(query).
				WithArgs("t1", "default_rate_limit_per_minute", "default_rate_limit_per_day", "default_rate_limit_scope").
				WillReturnRows(sqlmock.NewRows([]string{
					"default_rate_limit_per_minute",
					"default_rate_limit_per_day",
					"default_rate_limit_scope",
					"system_per_minute",
					"system_per_day",
					"system_scope",
				}).AddRow(toDriverValues(tc.row)...))

			st := New(db)
			config, err := st.GetEffectiveRateLimitConfig(context.Background(), "t1")
			require.NoError(t, err)
			require.Equal(t, tc.expected, config)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStoreIncrementRateLimitCounter(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		query := regexp.QuoteMeta(`INSERT INTO rbitr.rate_limit_counters (
			tenant_id, agent_id, tool_id, action_type, window, bucket_start, count, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7)
		ON CONFLICT (tenant_id, agent_id, tool_id, action_type, window, bucket_start)
		DO UPDATE SET
			count = rbitr.rate_limit_counters.count + 1,
			updated_at = EXCLUDED.updated_at
		WHERE rbitr.rate_limit_counters.count < $8
		RETURNING count`)

		now := time.Now().UTC()
		bucket := now.Truncate(time.Minute)

		mock.ExpectQuery(query).
			WithArgs("t1", "agent", "tool", "", "minute", bucket, now, int64(60)).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

		st := New(db)
		allowed, count, err := st.IncrementRateLimitCounter(context.Background(), "t1", "agent", "tool", "", "minute", bucket, now, 60)
		require.NoError(t, err)
		require.True(t, allowed)
		require.Equal(t, int64(1), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exceeded", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		query := regexp.QuoteMeta(`INSERT INTO rbitr.rate_limit_counters (
			tenant_id, agent_id, tool_id, action_type, window, bucket_start, count, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7)
		ON CONFLICT (tenant_id, agent_id, tool_id, action_type, window, bucket_start)
		DO UPDATE SET
			count = rbitr.rate_limit_counters.count + 1,
			updated_at = EXCLUDED.updated_at
		WHERE rbitr.rate_limit_counters.count < $8
		RETURNING count`)

		now := time.Now().UTC()
		bucket := now.Truncate(time.Minute)

		mock.ExpectQuery(query).
			WithArgs("t1", "agent", "tool", "", "minute", bucket, now, int64(1)).
			WillReturnError(sql.ErrNoRows)

		st := New(db)
		allowed, count, err := st.IncrementRateLimitCounter(context.Background(), "t1", "agent", "tool", "", "minute", bucket, now, 1)
		require.NoError(t, err)
		require.False(t, allowed)
		require.Equal(t, int64(0), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
