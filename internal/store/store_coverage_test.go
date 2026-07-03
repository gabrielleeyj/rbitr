package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestStoreCreatePolicyVersionStructured(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// admin write lock check: no row => writes allowed.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs("admin_write_lock").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes, structured_json, authoring_mode)`)).
		WithArgs("t1", "p1", "module", sqlmock.AnyArg(), "admin_1", "notes", []byte(`{"a":1}`), AuthoringModeStructured).
		WillReturnResult(sqlmock.NewResult(1, 1))

	st := New(db)
	err = st.CreatePolicyVersionStructured(context.Background(), "t1", "p1", "module", []byte(`{"a":1}`), "admin_1", "notes")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetPolicyVersionIncludesStructured(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"tenant_id", "policy_version", "rego_module", "created_at", "created_by", "notes", "structured_json", "authoring_mode"}).
		AddRow("t1", "p1", "module", time.Now().UTC(), "admin_1", "notes", []byte(`{"schema_version":"1"}`), AuthoringModeStructured)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+policyVersionColumns)).
		WithArgs("t1", "p1").
		WillReturnRows(rows)

	st := New(db)
	version, err := st.GetPolicyVersion(context.Background(), "t1", "p1")
	require.NoError(t, err)
	require.Equal(t, AuthoringModeStructured, version.AuthoringMode)
	require.JSONEq(t, `{"schema_version":"1"}`, string(version.StructuredJSON))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreGetPolicyVersionDefaultsAuthoringMode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// NULL structured_json and empty authoring_mode => defaults to rego.
	rows := sqlmock.NewRows([]string{"tenant_id", "policy_version", "rego_module", "created_at", "created_by", "notes", "structured_json", "authoring_mode"}).
		AddRow("t1", "p1", "module", time.Now().UTC(), nil, nil, nil, nil)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+policyVersionColumns)).
		WithArgs("t1", "p1").
		WillReturnRows(rows)

	st := New(db)
	version, err := st.GetPolicyVersion(context.Background(), "t1", "p1")
	require.NoError(t, err)
	require.Equal(t, AuthoringModeRego, version.AuthoringMode)
	require.Nil(t, version.StructuredJSON)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListFallbackHitPairs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Now().UTC().Add(-24 * time.Hour)
	rows := sqlmock.NewRows([]string{"tool_id", "action_type", "action_risk", "decision", "rule_id", "hit_count", "last_seen"}).
		AddRow("jira", "TICKET.CREATE", "LOW", "DENY", "rule_default_deny", 3, time.Now().UTC())

	mock.ExpectQuery(regexp.QuoteMeta(`FROM rbitr.action_decisions`)).
		WithArgs("t1", StringArray{"rule_default_deny"}, since, 50).
		WillReturnRows(rows)

	st := New(db)
	hits, err := st.ListFallbackHitPairs(context.Background(), "t1", []string{"rule_default_deny"}, since, 50)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "jira", hits[0].ToolID)
	require.Equal(t, 3, hits[0].HitCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreListUnusedActiveTools(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{"tool_id"}).AddRow("alpha").AddRow("beta")
	mock.ExpectQuery(regexp.QuoteMeta(`FROM rbitr.tools t`)).
		WithArgs("t1").
		WillReturnRows(rows)

	st := New(db)
	tools, err := st.ListUnusedActiveTools(context.Background(), "t1")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, tools)
	require.NoError(t, mock.ExpectationsWereMet())
}
