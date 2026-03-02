package setup

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInitializeRequiresIdempotencyWhenEnabled(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	service := NewService(db, Options{IdempotencyRequired: true})
	_, err = service.Initialize(context.Background(), &InitializeRequest{
		TenantName: "Acme",
	})
	require.ErrorIs(t, err, ErrIdempotencyRequired)
}

func TestInitializeReplayFromIdempotencyStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	req := InitializeRequest{
		TenantName:     "Acme",
		TenantID:       "t_acme",
		AdminKey:       "StrongAdminKey!2026",
		TenantKey:      "StrongTenantKey!2026",
		IdempotencyKey: "idem-1",
	}
	payloadHash := initializePayloadHash(&req)
	responseJSON := `{"bootstrap_complete":true,"tenant_id":"t_acme","tenant_name":"Acme","tenant_key_id":"tk_1","tenant_key":"tenant_key","tenant_key_created":false,"admin_key_id":"admin_1","admin_key":"admin_key","admin_key_created":false,"policy_version":"p_v1"}`

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT payload_hash, response_json
		FROM rbitr.setup_initialize_idempotency
		WHERE idempotency_key = $1`)).
		WithArgs("idem-1").
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "response_json"}).AddRow(payloadHash, []byte(responseJSON)))

	service := NewService(db, Options{IdempotencyRequired: true})
	resp, err := service.Initialize(context.Background(), &req)
	require.NoError(t, err)
	require.Equal(t, "t_acme", resp.TenantID)
	require.Equal(t, "p_v1", resp.PolicyVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializeIdempotencyConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	req := InitializeRequest{
		TenantName:     "Acme",
		TenantID:       "t_acme",
		IdempotencyKey: "idem-1",
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT payload_hash, response_json
		FROM rbitr.setup_initialize_idempotency
		WHERE idempotency_key = $1`)).
		WithArgs("idem-1").
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "response_json"}).AddRow("different_hash", []byte(`{}`)))

	service := NewService(db, Options{IdempotencyRequired: true})
	_, err = service.Initialize(context.Background(), &req)
	require.ErrorIs(t, err, ErrIdempotencyConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializeConcurrentLockConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
		to_regclass('rbitr.tenants') IS NOT NULL,
		to_regclass('rbitr.tenant_keys') IS NOT NULL,
		to_regclass('rbitr.admin_keys') IS NOT NULL,
		to_regclass('rbitr.policy_versions') IS NOT NULL,
		to_regclass('rbitr.tenant_config') IS NOT NULL,
		to_regclass('rbitr.system_settings') IS NOT NULL,
		to_regclass('rbitr.setup_state') IS NOT NULL,
		to_regclass('rbitr.setup_initialize_idempotency') IS NOT NULL`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenants",
			"tenant_keys",
			"admin_keys",
			"policy_versions",
			"tenant_config",
			"system_settings",
			"setup_state",
			"setup_initialize_idempotency",
		}).AddRow(true, true, true, true, true, true, true, true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_xact_lock($1)`)).
		WithArgs(setupInitializeLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(false))
	mock.ExpectRollback()

	service := NewService(db)
	_, err = service.Initialize(context.Background(), &InitializeRequest{TenantName: "Acme"})
	require.ErrorIs(t, err, ErrSetupInProgress)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatusIncludesSetupLifecycleState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
		to_regclass('rbitr.tenants') IS NOT NULL,
		to_regclass('rbitr.tenant_keys') IS NOT NULL,
		to_regclass('rbitr.admin_keys') IS NOT NULL,
		to_regclass('rbitr.policy_versions') IS NOT NULL,
		to_regclass('rbitr.tenant_config') IS NOT NULL,
		to_regclass('rbitr.system_settings') IS NOT NULL,
		to_regclass('rbitr.setup_state') IS NOT NULL,
		to_regclass('rbitr.setup_initialize_idempotency') IS NOT NULL`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenants",
			"tenant_keys",
			"admin_keys",
			"policy_versions",
			"tenant_config",
			"system_settings",
			"setup_state",
			"setup_initialize_idempotency",
		}).AddRow(true, true, true, true, true, true, true, true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM rbitr.system_settings WHERE key = $1`)).
		WithArgs(setupBootstrapKey).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT state, COALESCE(last_error, '') FROM rbitr.setup_state WHERE singleton = TRUE`)).
		WillReturnRows(sqlmock.NewRows([]string{"state", "last_error"}).AddRow(setupStateFailed, "boom"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM rbitr.admin_keys`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM rbitr.tenants`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	service := NewService(db, Options{IdempotencyRequired: true})
	status, err := service.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, setupStateFailed, status.SetupState)
	require.Equal(t, "boom", status.LastError)
	require.True(t, status.InitializeTokenRequired)
	require.True(t, status.IdempotencyRequired)
	require.NoError(t, mock.ExpectationsWereMet())
}
