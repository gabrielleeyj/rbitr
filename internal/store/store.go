package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrBootstrapComplete = errors.New("bootstrap already completed")
	ErrAdminWriteLocked  = errors.New("admin writes locked")
)

const (
	bootstrapKey      = "bootstrap_complete"
	adminWriteLockKey = "admin_write_lock"
	settingTrue       = "true"
	settingFalse      = "false"
)

type StoreAPI interface {
	GetTenantByKeyHash(ctx context.Context, keyHash string) (models.Tenant, error)
	GetAdminKeyByHash(ctx context.Context, keyHash string) (models.AdminKey, error)
	ListTenants(ctx context.Context) ([]models.TenantSummary, error)
	GetTenant(ctx context.Context, tenantID string) (models.TenantSummary, error)
	GetTenantKeyHash(ctx context.Context, tenantID string) (string, error)
	GetTool(ctx context.Context, tenantID, toolID string) (models.Tool, error)
	ListTools(ctx context.Context, tenantID string) ([]models.Tool, error)
	GetPolicy(ctx context.Context, tenantID string) (models.Policy, error)
	GetTenantConfig(ctx context.Context, tenantID string) (models.TenantConfig, error)
	ListPolicyVersions(ctx context.Context, tenantID string) ([]models.PolicyVersion, error)
	GetPolicyVersion(ctx context.Context, tenantID, policyVersion string) (models.PolicyVersion, error)
	CreatePolicyVersion(ctx context.Context, tenantID, policyVersion, regoModule, createdBy, notes string) error
	PublishPolicyVersion(ctx context.Context, tenantID, policyVersion string) error
	RollbackPolicyVersion(ctx context.Context, tenantID, policyVersion string) error
	GetRiskOverride(ctx context.Context, tenantID, actionType string) (string, error)
	ListRiskOverrides(ctx context.Context, tenantID string) ([]models.RiskOverride, error)
	DeleteRiskOverride(ctx context.Context, tenantID, actionType string) error
	InsertADR(ctx context.Context, record models.ActionDecisionRecord) error
	InsertApprovalRequest(ctx context.Context, req models.ApprovalRequest) error
	ListEvidence(ctx context.Context, tenantID string, limit int) ([]models.ActionDecisionRecord, error)
	ListEvidenceFiltered(ctx context.Context, tenantID, decision, actionType, risk string, since *time.Time, limit int) ([]models.ActionDecisionRecord, error)
	UpdateTenantConfig(ctx context.Context, tenantID, name, tenantKey string) error
	UpdateToolConfig(ctx context.Context, tenantID, toolID, baseURL, authType, authValue string) error
	UpdatePolicy(ctx context.Context, tenantID, regoModule, policyVersion string) error
	UpdateRiskOverride(ctx context.Context, tenantID, actionType, actionRisk string) error
	MarkBootstrapComplete(ctx context.Context) error
	GetBootstrapComplete(ctx context.Context) (bool, error)
	SetAdminWriteLock(ctx context.Context, locked bool) error
	GetAdminWriteLock(ctx context.Context) (bool, error)
	ListAuditEvents(ctx context.Context, tenantID string, limit int) ([]models.AdminAuditEvent, error)
	InsertAuditEvent(ctx context.Context, event models.AdminAuditEvent) error
}

// Store wraps database operations.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) StoreAPI {
	return &Store{db: db}
}

func (s *Store) GetTenantByKeyHash(ctx context.Context, keyHash string) (models.Tenant, error) {
	var tenant models.Tenant
	query := `SELECT t.tenant_id, t.name
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1`
	row := s.db.QueryRowContext(ctx, query, keyHash)
	if err := row.Scan(&tenant.TenantID, &tenant.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Tenant{}, ErrNotFound
		}
		return models.Tenant{}, err
	}
	return tenant, nil
}

func (s *Store) GetAdminKeyByHash(ctx context.Context, keyHash string) (models.AdminKey, error) {
	var key models.AdminKey
	var scopes StringArray
	query := `SELECT admin_key_id, key_hash, scopes FROM rbitr.admin_keys WHERE key_hash = $1`
	row := s.db.QueryRowContext(ctx, query, keyHash)
	if err := row.Scan(&key.AdminKeyID, &key.KeyHash, &scopes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.AdminKey{}, ErrNotFound
		}
		return models.AdminKey{}, err
	}
	key.Scopes = scopes
	return key, nil
}

func (s *Store) ListTenants(ctx context.Context) ([]models.TenantSummary, error) {
	query := `SELECT t.tenant_id, t.name, tc.active_policy_version, COALESCE(tool_counts.tool_count, 0)
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN (
			SELECT tenant_id, COUNT(*) AS tool_count
			FROM rbitr.tools
			GROUP BY tenant_id
		) tool_counts ON tool_counts.tenant_id = t.tenant_id
		ORDER BY t.tenant_id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []models.TenantSummary
	for rows.Next() {
		var tenant models.TenantSummary
		if err := rows.Scan(&tenant.TenantID, &tenant.Name, &tenant.ActivePolicyVersion, &tenant.ToolCount); err != nil {
			return nil, err
		}
		tenants = append(tenants, tenant)
	}
	return tenants, rows.Err()
}

func (s *Store) GetTenant(ctx context.Context, tenantID string) (models.TenantSummary, error) {
	query := `SELECT t.tenant_id, t.name, tc.active_policy_version, COALESCE(tool_counts.tool_count, 0)
		FROM rbitr.tenants t
		LEFT JOIN rbitr.tenant_config tc ON tc.tenant_id = t.tenant_id
		LEFT JOIN (
			SELECT tenant_id, COUNT(*) AS tool_count
			FROM rbitr.tools
			GROUP BY tenant_id
		) tool_counts ON tool_counts.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1`
	row := s.db.QueryRowContext(ctx, query, tenantID)
	var tenant models.TenantSummary
	if err := row.Scan(&tenant.TenantID, &tenant.Name, &tenant.ActivePolicyVersion, &tenant.ToolCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TenantSummary{}, ErrNotFound
		}
		return models.TenantSummary{}, err
	}
	return tenant, nil
}

func (s *Store) GetTenantKeyHash(ctx context.Context, tenantID string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT key_hash FROM rbitr.tenant_keys WHERE tenant_id = $1`, tenantID)
	var keyHash string
	if err := row.Scan(&keyHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return keyHash, nil
}

func (s *Store) GetTool(ctx context.Context, tenantID, toolID string) (models.Tool, error) {
	var tool models.Tool
	query := `SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, toolID)
	if err := row.Scan(&tool.ToolID, &tool.TenantID, &tool.BaseURL, &tool.AuthType, &tool.AuthValue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Tool{}, ErrNotFound
		}
		return models.Tool{}, err
	}
	return tool, nil
}

func (s *Store) ListTools(ctx context.Context, tenantID string) ([]models.Tool, error) {
	query := `SELECT tool_id, tenant_id, base_url, auth_type, auth_value FROM rbitr.tools WHERE tenant_id = $1 ORDER BY tool_id`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []models.Tool
	for rows.Next() {
		var tool models.Tool
		if err := rows.Scan(&tool.ToolID, &tool.TenantID, &tool.BaseURL, &tool.AuthType, &tool.AuthValue); err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, rows.Err()
}

func (s *Store) GetPolicy(ctx context.Context, tenantID string) (models.Policy, error) {
	var policy models.Policy
	query := `SELECT pv.policy_version, pv.tenant_id, pv.rego_module, pv.created_at
		FROM rbitr.tenant_config tc
		JOIN rbitr.policy_versions pv
			ON pv.tenant_id = tc.tenant_id
			AND pv.policy_version = tc.active_policy_version
		WHERE tc.tenant_id = $1`
	row := s.db.QueryRowContext(ctx, query, tenantID)
	var createdAt time.Time
	if err := row.Scan(&policy.PolicyVersion, &policy.TenantID, &policy.RegoModule, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Policy{}, ErrNotFound
		}
		return models.Policy{}, err
	}
	policy.PolicyID = policy.PolicyVersion
	policy.UpdatedAt = createdAt
	return policy, nil
}

func (s *Store) GetTenantConfig(ctx context.Context, tenantID string) (models.TenantConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, active_policy_version, created_at, updated_at FROM rbitr.tenant_config WHERE tenant_id = $1`, tenantID)
	var config models.TenantConfig
	if err := row.Scan(&config.TenantID, &config.ActivePolicyVersion, &config.CreatedAt, &config.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TenantConfig{}, ErrNotFound
		}
		return models.TenantConfig{}, err
	}
	return config, nil
}

func (s *Store) ListPolicyVersions(ctx context.Context, tenantID string) ([]models.PolicyVersion, error) {
	query := `SELECT tenant_id, policy_version, rego_module, created_at, created_by, notes
		FROM rbitr.policy_versions
		WHERE tenant_id = $1
		ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []models.PolicyVersion
	for rows.Next() {
		var version models.PolicyVersion
		if err := rows.Scan(&version.TenantID, &version.PolicyVersion, &version.RegoModule, &version.CreatedAt, &version.CreatedBy, &version.Notes); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) GetPolicyVersion(ctx context.Context, tenantID, policyVersion string) (models.PolicyVersion, error) {
	query := `SELECT tenant_id, policy_version, rego_module, created_at, created_by, notes
		FROM rbitr.policy_versions
		WHERE tenant_id = $1 AND policy_version = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, policyVersion)
	var version models.PolicyVersion
	if err := row.Scan(&version.TenantID, &version.PolicyVersion, &version.RegoModule, &version.CreatedAt, &version.CreatedBy, &version.Notes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.PolicyVersion{}, ErrNotFound
		}
		return models.PolicyVersion{}, err
	}
	return version, nil
}

func (s *Store) CreatePolicyVersion(ctx context.Context, tenantID, policyVersion, regoModule, createdBy, notes string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, policyVersion, regoModule, time.Now().UTC(), createdBy, notes)
	return err
}

func (s *Store) PublishPolicyVersion(ctx context.Context, tenantID, policyVersion string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	var exists bool
	row := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rbitr.policy_versions WHERE tenant_id = $1 AND policy_version = $2)`, tenantID, policyVersion)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET active_policy_version = $2, updated_at = $4`,
		tenantID, policyVersion, time.Now().UTC(), time.Now().UTC())
	return err
}

func (s *Store) RollbackPolicyVersion(ctx context.Context, tenantID, policyVersion string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	if policyVersion != "" {
		return s.PublishPolicyVersion(ctx, tenantID, policyVersion)
	}

	var active string
	row := s.db.QueryRowContext(ctx, `SELECT active_policy_version FROM rbitr.tenant_config WHERE tenant_id = $1`, tenantID)
	if err := row.Scan(&active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	row = s.db.QueryRowContext(ctx, `SELECT pv.policy_version
		FROM rbitr.policy_versions pv
		WHERE pv.tenant_id = $1
			AND pv.policy_version <> $2
		ORDER BY pv.created_at DESC
		LIMIT 1`, tenantID, active)
	var previous string
	if err := row.Scan(&previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.PublishPolicyVersion(ctx, tenantID, previous)
}

func (s *Store) GetRiskOverride(ctx context.Context, tenantID, actionType string) (string, error) {
	var risk string
	query := `SELECT action_risk FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, actionType)
	if err := row.Scan(&risk); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return risk, nil
}

func (s *Store) ListRiskOverrides(ctx context.Context, tenantID string) ([]models.RiskOverride, error) {
	query := `SELECT tenant_id, action_type, action_risk, updated_at
		FROM rbitr.action_risk_overrides
		WHERE tenant_id = $1
		ORDER BY action_type`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []models.RiskOverride
	for rows.Next() {
		var override models.RiskOverride
		if err := rows.Scan(&override.TenantID, &override.ActionType, &override.ActionRisk, &override.UpdatedAt); err != nil {
			return nil, err
		}
		overrides = append(overrides, override)
	}
	return overrides, rows.Err()
}

func (s *Store) DeleteRiskOverride(ctx context.Context, tenantID, actionType string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`, tenantID, actionType)
	return err
}

//nolint:gocritic // API favors value records for test setup convenience.
func (s *Store) InsertADR(ctx context.Context, record models.ActionDecisionRecord) error {
	reasonsJSON, err := json.Marshal(record.Reasons)
	if err != nil {
		return err
	}
	constraintsJSON, err := json.Marshal(record.Constraints)
	if err != nil {
		return err
	}
	tags := StringArray(record.Tags)

	query := `INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`
	_, err = s.db.ExecContext(ctx, query,
		record.DecisionID,
		record.RequestID,
		record.TenantID,
		record.AgentID,
		record.ToolID,
		record.ActionType,
		record.ActionRisk,
		record.ActionSummary,
		record.Decision,
		record.DecisionVersion,
		record.DecisionRisk,
		record.RuleID,
		record.RulePriority,
		reasonsJSON,
		constraintsJSON,
		tags,
		record.PolicyVersion,
		record.Reason,
		record.RequestHash,
		record.ResponseHash,
		record.ApprovalRequestID,
		record.CreatedAt,
	)
	return err
}

//nolint:gocritic // API favors value records for test setup convenience.
func (s *Store) InsertApprovalRequest(ctx context.Context, req models.ApprovalRequest) error {
	query := `INSERT INTO rbitr.approval_requests (
		approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash,
		status, expires_at, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := s.db.ExecContext(ctx, query,
		req.ApprovalRequestID,
		req.TenantID,
		req.AgentID,
		req.ToolID,
		req.ActionType,
		req.RequestHash,
		req.Status,
		req.ExpiresAt,
		req.CreatedAt,
	)
	return err
}

func (s *Store) ListEvidence(ctx context.Context, tenantID string, limit int) ([]models.ActionDecisionRecord, error) {
	query := `SELECT decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
		FROM rbitr.action_decisions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2`
	rows, err := s.db.QueryContext(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.ActionDecisionRecord
	for rows.Next() {
		var record models.ActionDecisionRecord
		var reasonsJSON []byte
		var constraintsJSON []byte
		var tags StringArray
		if err := rows.Scan(
			&record.DecisionID,
			&record.RequestID,
			&record.TenantID,
			&record.AgentID,
			&record.ToolID,
			&record.ActionType,
			&record.ActionRisk,
			&record.ActionSummary,
			&record.Decision,
			&record.DecisionVersion,
			&record.DecisionRisk,
			&record.RuleID,
			&record.RulePriority,
			&reasonsJSON,
			&constraintsJSON,
			&tags,
			&record.PolicyVersion,
			&record.Reason,
			&record.RequestHash,
			&record.ResponseHash,
			&record.ApprovalRequestID,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(reasonsJSON, &record.Reasons); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(constraintsJSON, &record.Constraints); err != nil {
			return nil, err
		}
		record.Tags = []string(tags)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) ListEvidenceFiltered(ctx context.Context, tenantID, decision, actionType, risk string, since *time.Time, limit int) ([]models.ActionDecisionRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []any{tenantID}
	clauses := []string{"tenant_id = $1"}

	if decision != "" {
		args = append(args, decision)
		clauses = append(clauses, fmt.Sprintf("decision = $%d", len(args)))
	}
	if actionType != "" {
		args = append(args, actionType)
		clauses = append(clauses, fmt.Sprintf("action_type = $%d", len(args)))
	}
	if risk != "" {
		args = append(args, risk)
		clauses = append(clauses, fmt.Sprintf("action_risk = $%d", len(args)))
	}
	if since != nil {
		args = append(args, *since)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}

	args = append(args, limit)
	query := fmt.Sprintf(`SELECT decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, decision_version, decision_risk, rule_id, rule_priority,
		reasons, constraints, tags, policy_version, reason, request_hash,
		response_hash, approval_request_id, created_at
		FROM rbitr.action_decisions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d`, strings.Join(clauses, " AND "), len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.ActionDecisionRecord
	for rows.Next() {
		var record models.ActionDecisionRecord
		var reasonsJSON []byte
		var constraintsJSON []byte
		var tags StringArray
		if err := rows.Scan(
			&record.DecisionID,
			&record.RequestID,
			&record.TenantID,
			&record.AgentID,
			&record.ToolID,
			&record.ActionType,
			&record.ActionRisk,
			&record.ActionSummary,
			&record.Decision,
			&record.DecisionVersion,
			&record.DecisionRisk,
			&record.RuleID,
			&record.RulePriority,
			&reasonsJSON,
			&constraintsJSON,
			&tags,
			&record.PolicyVersion,
			&record.Reason,
			&record.RequestHash,
			&record.ResponseHash,
			&record.ApprovalRequestID,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(reasonsJSON, &record.Reasons); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(constraintsJSON, &record.Constraints); err != nil {
			return nil, err
		}
		record.Tags = []string(tags)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) UpdateTenantConfig(ctx context.Context, tenantID, name, tenantKey string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	keyHash := hashKey(tenantKey)
	if name != "" {
		if _, err := s.db.ExecContext(ctx, `UPDATE rbitr.tenants SET name = $1 WHERE tenant_id = $2`, name, tenantID); err != nil {
			return err
		}
	}
	if tenantKey != "" {
		if _, err := s.db.ExecContext(ctx, `UPDATE rbitr.tenant_keys SET key_hash = $1 WHERE tenant_id = $2`, keyHash, tenantID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) UpdateToolConfig(ctx context.Context, tenantID, toolID, baseURL, authType, authValue string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `UPDATE rbitr.tools SET base_url = $1, auth_type = $2, auth_value = $3 WHERE tenant_id = $4 AND tool_id = $5`,
		baseURL, authType, authValue, tenantID, toolID)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) UpdatePolicy(ctx context.Context, tenantID, regoModule, policyVersion string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, notes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, policy_version) DO UPDATE SET rego_module = $3`,
		tenantID, policyVersion, regoModule, time.Now().UTC(), "legacy admin policy update")
	if err != nil {
		return err
	}
	return s.PublishPolicyVersion(ctx, tenantID, policyVersion)
}

func (s *Store) UpdateRiskOverride(ctx context.Context, tenantID, actionType, actionRisk string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.action_risk_overrides (tenant_id, action_type, action_risk, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, action_type) DO UPDATE SET action_risk = $3, updated_at = $4`,
		tenantID, actionType, actionRisk, time.Now().UTC())
	return err
}

func (s *Store) MarkBootstrapComplete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
		bootstrapKey, settingTrue, time.Now().UTC())
	return err
}

func (s *Store) GetBootstrapComplete(ctx context.Context) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, bootstrapKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return value == settingTrue, nil
}

func (s *Store) SetAdminWriteLock(ctx context.Context, locked bool) error {
	value := settingFalse
	if locked {
		value = settingTrue
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
		adminWriteLockKey, value, time.Now().UTC())
	return err
}

func (s *Store) GetAdminWriteLock(ctx context.Context) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, adminWriteLockKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return value == settingTrue, nil
}

func (s *Store) ensureAdminWritesAllowed(ctx context.Context) error {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, adminWriteLockKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if value == settingTrue {
		return ErrAdminWriteLocked
	}
	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, tenantID string, limit int) ([]models.AdminAuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []any{limit}
	query := `SELECT audit_event_id, tenant_id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
		FROM rbitr.admin_audit_events`
	if tenantID != "" {
		query += ` WHERE tenant_id = $2`
		args = append(args, tenantID)
	}
	query += ` ORDER BY created_at DESC LIMIT $1`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AdminAuditEvent
	for rows.Next() {
		var event models.AdminAuditEvent
		if err := rows.Scan(
			&event.AuditEventID,
			&event.TenantID,
			&event.ActorType,
			&event.ActorID,
			&event.ActorDisplay,
			&event.Action,
			&event.ResourceType,
			&event.ResourceID,
			&event.Before,
			&event.After,
			&event.RequestID,
			&event.IP,
			&event.UserAgent,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) InsertAuditEvent(ctx context.Context, event models.AdminAuditEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		event.AuditEventID,
		event.TenantID,
		event.ActorType,
		event.ActorID,
		event.ActorDisplay,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		event.Before,
		event.After,
		event.RequestID,
		event.IP,
		event.UserAgent,
		event.CreatedAt,
	)
	return err
}

func hashKey(key string) string {
	return utils.HashString(key)
}
