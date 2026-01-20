package store

import (
	"context"
	"database/sql"
	"errors"
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
	GetTool(ctx context.Context, tenantID, toolID string) (models.Tool, error)
	GetPolicy(ctx context.Context, tenantID string) (models.Policy, error)
	GetRiskOverride(ctx context.Context, tenantID, actionType string) (string, error)
	InsertADR(ctx context.Context, record models.ActionDecisionRecord) error
	InsertApprovalRequest(ctx context.Context, req models.ApprovalRequest) error
	ListEvidence(ctx context.Context, tenantID string, limit int) ([]models.ActionDecisionRecord, error)
	UpdateTenantConfig(ctx context.Context, tenantID, name, tenantKey string) error
	UpdateToolConfig(ctx context.Context, tenantID, toolID, baseURL, authType, authValue string) error
	UpdatePolicy(ctx context.Context, tenantID, regoModule, policyVersion string) error
	UpdateRiskOverride(ctx context.Context, tenantID, actionType, actionRisk string) error
	MarkBootstrapComplete(ctx context.Context) error
	SetAdminWriteLock(ctx context.Context, locked bool) error
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

func (s *Store) GetPolicy(ctx context.Context, tenantID string) (models.Policy, error) {
	var policy models.Policy
	query := `SELECT policy_id, tenant_id, rego_module, policy_version, updated_at FROM rbitr.policies WHERE tenant_id = $1`
	row := s.db.QueryRowContext(ctx, query, tenantID)
	if err := row.Scan(&policy.PolicyID, &policy.TenantID, &policy.RegoModule, &policy.PolicyVersion, &policy.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Policy{}, ErrNotFound
		}
		return models.Policy{}, err
	}
	return policy, nil
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

//nolint:gocritic // API favors value records for test setup convenience.
func (s *Store) InsertADR(ctx context.Context, record models.ActionDecisionRecord) error {
	query := `INSERT INTO rbitr.action_decisions (
		decision_id, request_id, tenant_id, agent_id, tool_id, action_type, action_risk,
		action_summary, decision, reason, rule_id, policy_version, request_hash,
		response_hash, approval_request_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`
	_, err := s.db.ExecContext(ctx, query,
		record.DecisionID,
		record.RequestID,
		record.TenantID,
		record.AgentID,
		record.ToolID,
		record.ActionType,
		record.ActionRisk,
		record.ActionSummary,
		record.Decision,
		record.Reason,
		record.RuleID,
		record.PolicyVersion,
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
		action_summary, decision, reason, rule_id, policy_version, request_hash,
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
			&record.Reason,
			&record.RuleID,
			&record.PolicyVersion,
			&record.RequestHash,
			&record.ResponseHash,
			&record.ApprovalRequestID,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
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

	_, err := s.db.ExecContext(ctx, `UPDATE rbitr.policies SET rego_module = $1, policy_version = $2, updated_at = $3 WHERE tenant_id = $4`,
		regoModule, policyVersion, time.Now().UTC(), tenantID)
	if err != nil {
		return err
	}

	return nil
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

func hashKey(key string) string {
	return utils.HashString(key)
}
