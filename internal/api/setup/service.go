package setup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrSetupComplete  = errors.New("setup already complete")
	ErrSchemaNotReady = errors.New("schema not ready")
	ErrInvalidRequest = errors.New("invalid setup request")
	tenantIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{3,64}$`)
)

const (
	adminKeyIDPattern      = "admin_%s"
	defaultPolicyNotes     = "Initialized by setup workflow"
	setupBootstrapKey      = "bootstrap_complete"
	setupAdminWriteLockKey = "admin_write_lock"

	defaultPolicyVersion = "p_v1"

	defaultApprovalTTLSeconds = "900"
	defaultAuditRetentionDays = "365"
	defaultRateLimitPerMinute = "60"
	defaultRateLimitPerDay    = "10000"
	defaultRateLimitScope     = "tenant_agent_tool"

	defaultBooleanFalse = "false"

	adminKeyPrefix      = "rbtr_admin_"
	defaultPolicyModule = `package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
	"version": "2026-01-20",
	"decision": decision,
	"risk": risk,
	"rule": {"id": rule_id, "priority": priority},
	"reasons": [{"code": code, "message": message}],
	"constraints": {},
	"tags": []
}

allow_actions := {
	"TICKET.CREATE",
	"TICKET.COMMENT",
	"TICKET.UPDATE",
	"CRM.READ",
	"DATA.READ",
	"DATA.QUERY"
}

require_approval_actions := {
	"PAYMENT.REFUND",
	"ACCESS.ROLE_CHANGE"
}

deny_actions := {
	"DATA.EXPORT",
	"DATA.BULK_EXPORT",
	"ACCESS.GRANT",
	"DATA.DELETE",
	"CRM.DELETE"
}

decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
	input.action_type
	deny_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
	input.action_type
	require_approval_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
	input.action_type
	allow_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
	input.action_risk == "HIGH"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_critical_risk_unknown", 80, "CRITICAL_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
	input.action_risk == "CRITICAL"
} else := decision_obj("DENY", "MEDIUM", "rule_default_deny", 100, "DEFAULT_DENY", "Default deny: no matching rule or missing required fields")
`
)

type StatusResponse struct {
	SetupRequired     bool `json:"setup_required"`
	BootstrapComplete bool `json:"bootstrap_complete"`
	DatabaseReachable bool `json:"database_reachable"`
	SchemaReady       bool `json:"schema_ready"`
	AdminKeyCount     int  `json:"admin_key_count"`
	TenantCount       int  `json:"tenant_count"`
}

type InitializeRequest struct {
	TenantName string `json:"tenant_name"`
	TenantID   string `json:"tenant_id"`
	AdminKey   string `json:"admin_key"`
	TenantKey  string `json:"tenant_key"`
}

type InitializeResponse struct {
	BootstrapComplete bool   `json:"bootstrap_complete"`
	TenantID          string `json:"tenant_id"`
	TenantName        string `json:"tenant_name"`
	TenantKeyID       string `json:"tenant_key_id"`
	TenantKey         string `json:"tenant_key"`
	TenantKeyCreated  bool   `json:"tenant_key_created"`
	AdminKeyID        string `json:"admin_key_id"`
	AdminKey          string `json:"admin_key"`
	AdminKeyCreated   bool   `json:"admin_key_created"`
	PolicyVersion     string `json:"policy_version"`
}

type Service interface {
	Status(ctx context.Context) (StatusResponse, error)
	Initialize(ctx context.Context, req InitializeRequest) (InitializeResponse, error)
}

type dbService struct {
	db *sql.DB
}

func NewService(db *sql.DB) Service {
	return &dbService{db: db}
}

func (s *dbService) Status(ctx context.Context) (StatusResponse, error) {
	status := StatusResponse{
		DatabaseReachable: true,
	}

	schemaReady, err := s.schemaReady(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	status.SchemaReady = schemaReady

	if !schemaReady {
		status.SetupRequired = true
		return status, nil
	}

	bootstrapComplete, err := getBootstrapComplete(ctx, s.db)
	if err != nil {
		return StatusResponse{}, err
	}
	status.BootstrapComplete = bootstrapComplete
	status.SetupRequired = !bootstrapComplete

	adminCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM rbitr.admin_keys`)
	if err != nil {
		return StatusResponse{}, err
	}
	tenantCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM rbitr.tenants`)
	if err != nil {
		return StatusResponse{}, err
	}
	status.AdminKeyCount = adminCount
	status.TenantCount = tenantCount

	return status, nil
}

func (s *dbService) Initialize(ctx context.Context, req InitializeRequest) (InitializeResponse, error) {
	tenantName := strings.TrimSpace(req.TenantName)
	if tenantName == "" {
		return InitializeResponse{}, fmt.Errorf("%w: tenant_name is required", ErrInvalidRequest)
	}
	if len(tenantName) > 120 { //nolint:mnd // ignore tenantName.
		return InitializeResponse{}, fmt.Errorf("%w: tenant_name too long", ErrInvalidRequest)
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = "t_" + uuid.NewString()[:8]
	}
	if !tenantIDPattern.MatchString(tenantID) {
		return InitializeResponse{}, fmt.Errorf("%w: tenant_id must match %s", ErrInvalidRequest, tenantIDPattern.String())
	}

	adminKey := strings.TrimSpace(req.AdminKey)
	adminKeyCreated := false
	if adminKey == "" {
		generated, err := generateSecret(adminKeyPrefix)
		if err != nil {
			return InitializeResponse{}, err
		}
		adminKey = generated
		adminKeyCreated = true
	}
	if len(adminKey) < 16 { //nolint:mnd // ignore this as its a character checker.
		return InitializeResponse{}, fmt.Errorf("%w: admin_key must be at least 16 characters", ErrInvalidRequest)
	}

	tenantKey := strings.TrimSpace(req.TenantKey)
	tenantKeyCreated := false
	tenantKeyHash := ""
	tenantKeyPrefix := ""
	if tenantKey == "" {
		generated, hash, prefix, err := utils.GenerateAPIKey()
		if err != nil {
			return InitializeResponse{}, err
		}
		tenantKey = generated
		tenantKeyHash = hash
		tenantKeyPrefix = prefix
		tenantKeyCreated = true
	} else {
		if len(tenantKey) < 16 { //nolint:mnd // ignore this as its a character checker.
			return InitializeResponse{}, fmt.Errorf("%w: tenant_key must be at least 16 characters", ErrInvalidRequest)
		}
		tenantKeyHash = utils.HashTenantKey(tenantKey)
		tenantKeyPrefix = keyPrefix(tenantKey)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InitializeResponse{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	schemaReady, err := schemaReadyWithQuerier(ctx, tx)
	if err != nil {
		return InitializeResponse{}, err
	}
	if !schemaReady {
		return InitializeResponse{}, ErrSchemaNotReady
	}

	bootstrapComplete, err := getBootstrapComplete(ctx, tx)
	if err != nil {
		return InitializeResponse{}, err
	}
	if bootstrapComplete {
		return InitializeResponse{}, ErrSetupComplete
	}

	now := time.Now().UTC()
	tenantKeyID := uuid.NewString()
	adminKeyID := fmt.Sprintf(adminKeyIDPattern, uuid.NewString()[:8])

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenants (tenant_id, name, enabled, created_at) VALUES ($1, $2, true, $3)`,
		tenantID, tenantName, now,
	); err != nil {
		return InitializeResponse{}, normalizeWriteError(err, "tenant_id")
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenant_keys (key_id, tenant_id, key_hash, key_prefix, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantKeyID, tenantID, tenantKeyHash, tenantKeyPrefix, now,
	); err != nil {
		return InitializeResponse{}, normalizeWriteError(err, "tenant_key")
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rbitr.admin_keys (admin_key_id, key_hash, scopes, created_at)
		 VALUES ($1, $2, $3, $4)`,
		adminKeyID, utils.HashString(adminKey), store.StringArray{"admin:read", "admin:write"}, now,
	); err != nil {
		return InitializeResponse{}, normalizeWriteError(err, "admin_key")
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, defaultPolicyVersion, defaultPolicyModule, now, "setup_wizard", defaultPolicyNotes,
	); err != nil {
		return InitializeResponse{}, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at, version)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (tenant_id) DO UPDATE
		 SET active_policy_version = $2, updated_at = $4, version = rbitr.tenant_config.version + 1`,
		tenantID, defaultPolicyVersion, now, now,
	); err != nil {
		return InitializeResponse{}, err
	}

	settingWrites := []struct {
		key   string
		value string
	}{
		{key: setupBootstrapKey, value: "true"},
		{key: setupAdminWriteLockKey, value: defaultBooleanFalse},
		{key: "default_approval_ttl_seconds", value: defaultApprovalTTLSeconds},
		{key: "audit_retention_days", value: defaultAuditRetentionDays},
		{key: "disable_x_tenant_key", value: defaultBooleanFalse},
		{key: "feature_rate_limiting", value: defaultBooleanFalse},
		{key: "feature_arg_constraints", value: defaultBooleanFalse},
		{key: "default_rate_limit_per_minute", value: defaultRateLimitPerMinute},
		{key: "default_rate_limit_per_day", value: defaultRateLimitPerDay},
		{key: "default_rate_limit_scope", value: defaultRateLimitScope},
	}
	for _, setting := range settingWrites {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rbitr.system_settings (key, value, updated_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
			setting.key, setting.value, now,
		); err != nil {
			return InitializeResponse{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return InitializeResponse{}, err
	}

	return InitializeResponse{
		BootstrapComplete: true,
		TenantID:          tenantID,
		TenantName:        tenantName,
		TenantKeyID:       tenantKeyID,
		TenantKey:         tenantKey,
		TenantKeyCreated:  tenantKeyCreated,
		AdminKeyID:        adminKeyID,
		AdminKey:          adminKey,
		AdminKeyCreated:   adminKeyCreated,
		PolicyVersion:     defaultPolicyVersion,
	}, nil
}

type queryRowScanner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type setupQuerier interface {
	queryRowScanner
}

func (s *dbService) schemaReady(ctx context.Context) (bool, error) {
	return schemaReadyWithQuerier(ctx, s.db)
}

func schemaReadyWithQuerier(ctx context.Context, q setupQuerier) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT
		to_regclass('rbitr.tenants') IS NOT NULL,
		to_regclass('rbitr.tenant_keys') IS NOT NULL,
		to_regclass('rbitr.admin_keys') IS NOT NULL,
		to_regclass('rbitr.policy_versions') IS NOT NULL,
		to_regclass('rbitr.tenant_config') IS NOT NULL,
		to_regclass('rbitr.system_settings') IS NOT NULL`)

	var hasTenants, hasTenantKeys, hasAdminKeys, hasPolicyVersions, hasTenantConfig, hasSystemSettings bool
	if err := row.Scan(
		&hasTenants,
		&hasTenantKeys,
		&hasAdminKeys,
		&hasPolicyVersions,
		&hasTenantConfig,
		&hasSystemSettings,
	); err != nil {
		return false, err
	}

	return hasTenants &&
		hasTenantKeys &&
		hasAdminKeys &&
		hasPolicyVersions &&
		hasTenantConfig &&
		hasSystemSettings, nil
}

func getBootstrapComplete(ctx context.Context, q setupQuerier) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, setupBootstrapKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func countRows(ctx context.Context, q setupQuerier, query string) (int, error) {
	row := q.QueryRowContext(ctx, query)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func generateSecret(prefix string) (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

//nolint:mnd // ignore trimming of keyPrefix.
func keyPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 14 {
		return trimmed
	}
	return trimmed[:14]
}

func normalizeWriteError(err error, field string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return fmt.Errorf("%w: %s already exists", ErrInvalidRequest, field)
	}
	return err
}
