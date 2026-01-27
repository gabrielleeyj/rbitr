package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrBootstrapComplete = errors.New("bootstrap already completed")
	ErrAdminWriteLocked  = errors.New("admin writes locked")
	ErrInvalidState      = errors.New("invalid state")
)

const (
	bootstrapKey                 = "bootstrap_complete"
	adminWriteLockKey            = "admin_write_lock"
	defaultApprovalTTLSecondsKey = "default_approval_ttl_seconds"
	settingTrue                  = "true"
	settingFalse                 = "false"
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
	ListApprovalRequests(ctx context.Context, tenantID, status string, limit, offset int) ([]models.ApprovalRequest, error)
	GetApprovalRequest(ctx context.Context, tenantID, approvalRequestID string) (models.ApprovalRequest, error)
	CountPendingApprovals(ctx context.Context, tenantID string, now time.Time) (int, error)
	ApproveApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	DenyApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	RevokeApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	MarkApprovalExecuted(ctx context.Context, tenantID, approvalRequestID, requestID, decisionID string, executedAt time.Time) error
	MarkApprovalExpired(ctx context.Context, tenantID, approvalRequestID string, expiredAt time.Time) error
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
	SetDefaultApprovalTTLSeconds(ctx context.Context, seconds int) error
	GetDefaultApprovalTTLSeconds(ctx context.Context) (int, error)
	ListAuditEvents(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string) ([]models.AdminAuditEvent, error)
	InsertAuditEvent(ctx context.Context, event models.AdminAuditEvent) error
	GetNotificationConfig(ctx context.Context, tenantID string) (models.NotificationConfig, error)
	UpsertNotificationConfig(ctx context.Context, config models.NotificationConfig) error
	ListMailingLists(ctx context.Context, tenantID string) ([]models.MailingList, error)
	GetMailingList(ctx context.Context, tenantID, mailingListID string) (models.MailingList, error)
	ListMailingListMembers(ctx context.Context, mailingListID string) ([]models.MailingListMember, error)
	CreateMailingList(ctx context.Context, list models.MailingList, members []string) error
	UpdateMailingList(ctx context.Context, list models.MailingList, members []string) error
	DeleteMailingList(ctx context.Context, tenantID, mailingListID string) error
	GetNotificationSuppression(ctx context.Context, dedupKey string) (models.NotificationSuppression, error)
	UpsertNotificationSuppression(ctx context.Context, suppression models.NotificationSuppression) error
	ListApprovalsExpiring(ctx context.Context, now time.Time, window time.Duration) ([]models.ApprovalRequest, error)
	ListApprovalsExpired(ctx context.Context, now time.Time) ([]models.ApprovalRequest, error)
	TryAdvisoryLock(ctx context.Context, key int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, key int64) error
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
		var createdBy sql.NullString
		var notes sql.NullString
		if err := rows.Scan(&version.TenantID, &version.PolicyVersion, &version.RegoModule, &version.CreatedAt, &createdBy, &notes); err != nil {
			return nil, err
		}
		if createdBy.Valid {
			version.CreatedBy = createdBy.String
		}
		if notes.Valid {
			version.Notes = notes.String
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
	var createdBy sql.NullString
	var notes sql.NullString
	if err := row.Scan(&version.TenantID, &version.PolicyVersion, &version.RegoModule, &version.CreatedAt, &createdBy, &notes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.PolicyVersion{}, ErrNotFound
		}
		return models.PolicyVersion{}, err
	}
	if createdBy.Valid {
		version.CreatedBy = createdBy.String
	}
	if notes.Valid {
		version.Notes = notes.String
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
		status, approval_token_hash, expires_at, created_at, policy_version,
		decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id,
		request_decision_id, action_summary, risk, rule_id, reasons
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`
	var reasonsJSON []byte
	if len(req.Reasons) > 0 {
		var err error
		reasonsJSON, err = json.Marshal(req.Reasons)
		if err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, query,
		req.ApprovalRequestID,
		req.TenantID,
		req.AgentID,
		req.ToolID,
		req.ActionType,
		req.RequestHash,
		req.Status,
		req.ApprovalTokenHash,
		req.ExpiresAt,
		req.CreatedAt,
		nullableString(req.PolicyVersion),
		req.DecidedAt,
		nullableString(req.DecidedBy),
		nullableString(req.DecisionComment),
		req.ExecutedAt,
		nullableString(req.ExecutedRequestID),
		nullableString(req.ExecutedDecisionID),
		nullableString(req.RequestDecisionID),
		nullableString(req.ActionSummary),
		nullableString(req.Risk),
		nullableString(req.RuleID),
		reasonsJSON,
	)
	return err
}

func (s *Store) ListApprovalRequests(ctx context.Context, tenantID, status string, limit, offset int) ([]models.ApprovalRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	clauses := []string{"tenant_id = $1"}
	if status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, limit, offset)

	query := fmt.Sprintf(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, reasons
		FROM rbitr.approval_requests
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, strings.Join(clauses, " AND "), len(args)-1, len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []models.ApprovalRequest
	for rows.Next() {
		approval, err := scanApprovalRequest(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func (s *Store) GetApprovalRequest(ctx context.Context, tenantID, approvalRequestID string) (models.ApprovalRequest, error) {
	row := s.db.QueryRowContext(ctx, `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, reasons
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`, tenantID, approvalRequestID)
	approval, err := scanApprovalRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ApprovalRequest{}, ErrNotFound
		}
		return models.ApprovalRequest{}, err
	}
	return approval, nil
}

func (s *Store) CountPendingApprovals(ctx context.Context, tenantID string, now time.Time) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM rbitr.approval_requests
		WHERE tenant_id = $1
		AND status = 'PENDING'
		AND expires_at > $2`, tenantID, now)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ApproveApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error {
	return s.updateApprovalDecision(ctx, tenantID, approvalRequestID, "APPROVED", decidedBy, comment, decidedAt)
}

func (s *Store) DenyApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error {
	return s.updateApprovalDecision(ctx, tenantID, approvalRequestID, "DENIED", decidedBy, comment, decidedAt)
}

func (s *Store) RevokeApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error {
	return s.updateApprovalDecision(ctx, tenantID, approvalRequestID, "REVOKED", decidedBy, comment, decidedAt)
}

func (s *Store) MarkApprovalExecuted(ctx context.Context, tenantID, approvalRequestID, requestID, decisionID string, executedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = 'EXECUTED', executed_at = $1, executed_request_id = $2, executed_decision_id = $3
		WHERE tenant_id = $4 AND approval_request_id = $5 AND status = 'APPROVED'`,
		executedAt, requestID, decisionID, tenantID, approvalRequestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return s.approvalStateError(ctx, tenantID, approvalRequestID)
	}
	return nil
}

func (s *Store) MarkApprovalExpired(ctx context.Context, tenantID, approvalRequestID string, expiredAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = 'EXPIRED', decided_at = $1
		WHERE tenant_id = $2 AND approval_request_id = $3 AND status IN ('PENDING','APPROVED')`,
		expiredAt, tenantID, approvalRequestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return s.approvalStateError(ctx, tenantID, approvalRequestID)
	}
	return nil
}

func (s *Store) updateApprovalDecision(ctx context.Context, tenantID, approvalRequestID, status, decidedBy, comment string, decidedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = $1, decided_at = $2, decided_by = $3, decision_comment = $4
		WHERE tenant_id = $5 AND approval_request_id = $6 AND status = 'PENDING'`,
		status, decidedAt, nullableString(decidedBy), nullableString(comment), tenantID, approvalRequestID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return s.approvalStateError(ctx, tenantID, approvalRequestID)
	}
	return nil
}

func (s *Store) approvalStateError(ctx context.Context, tenantID, approvalRequestID string) error {
	row := s.db.QueryRowContext(ctx, `SELECT status FROM rbitr.approval_requests WHERE tenant_id = $1 AND approval_request_id = $2`, tenantID, approvalRequestID)
	var status string
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return ErrInvalidState
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

func (s *Store) SetDefaultApprovalTTLSeconds(ctx context.Context, seconds int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		defaultApprovalTTLSecondsKey,
		strconv.Itoa(seconds),
		time.Now().UTC(),
	)
	return err
}

func (s *Store) GetDefaultApprovalTTLSeconds(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, defaultApprovalTTLSecondsKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
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

func (s *Store) ListAuditEvents(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string) ([]models.AdminAuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	clauses := []string{"1=1"}
	args := []any{}
	if tenantID != "" {
		clauses = append(clauses, fmt.Sprintf("tenant_id = $%d", len(args)+1))
		args = append(args, tenantID)
	}
	if action != "" {
		clauses = append(clauses, fmt.Sprintf("action = $%d", len(args)+1))
		args = append(args, action)
	}
	if resourceType != "" {
		clauses = append(clauses, fmt.Sprintf("resource_type = $%d", len(args)+1))
		args = append(args, resourceType)
	}
	if actorID != "" {
		clauses = append(clauses, fmt.Sprintf("actor_id = $%d", len(args)+1))
		args = append(args, actorID)
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(`SELECT audit_event_id, tenant_id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
		FROM rbitr.admin_audit_events
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, strings.Join(clauses, " AND "), len(args)-1, len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AdminAuditEvent
	for rows.Next() {
		var event models.AdminAuditEvent
		var tenantIDValue sql.NullString
		var actorID sql.NullString
		var actorDisplay sql.NullString
		var resourceID sql.NullString
		var beforeJSON []byte
		var afterJSON []byte
		var requestID sql.NullString
		var ip sql.NullString
		var userAgent sql.NullString
		if err := rows.Scan(
			&event.AuditEventID,
			&tenantIDValue,
			&event.ActorType,
			&actorID,
			&actorDisplay,
			&event.Action,
			&event.ResourceType,
			&resourceID,
			&beforeJSON,
			&afterJSON,
			&requestID,
			&ip,
			&userAgent,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if tenantIDValue.Valid {
			event.TenantID = tenantIDValue.String
		}
		if actorID.Valid {
			event.ActorID = actorID.String
		}
		if actorDisplay.Valid {
			event.ActorDisplay = actorDisplay.String
		}
		if resourceID.Valid {
			event.ResourceID = resourceID.String
		}
		if len(beforeJSON) > 0 {
			event.Before = beforeJSON
		}
		if len(afterJSON) > 0 {
			event.After = afterJSON
		}
		if requestID.Valid {
			event.RequestID = requestID.String
		}
		if ip.Valid {
			event.IP = ip.String
		}
		if userAgent.Valid {
			event.UserAgent = userAgent.String
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) InsertAuditEvent(ctx context.Context, event models.AdminAuditEvent) error {
	tenantID := sql.NullString{String: event.TenantID, Valid: event.TenantID != ""}
	actorID := sql.NullString{String: event.ActorID, Valid: event.ActorID != ""}
	actorDisplay := sql.NullString{String: event.ActorDisplay, Valid: event.ActorDisplay != ""}
	resourceID := sql.NullString{String: event.ResourceID, Valid: event.ResourceID != ""}
	requestID := sql.NullString{String: event.RequestID, Valid: event.RequestID != ""}
	ip := sql.NullString{String: event.IP, Valid: event.IP != ""}
	userAgent := sql.NullString{String: event.UserAgent, Valid: event.UserAgent != ""}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		event.AuditEventID,
		tenantID,
		event.ActorType,
		actorID,
		actorDisplay,
		event.Action,
		event.ResourceType,
		resourceID,
		event.Before,
		event.After,
		requestID,
		ip,
		userAgent,
		event.CreatedAt,
	)
	return err
}

func (s *Store) GetNotificationConfig(ctx context.Context, tenantID string) (models.NotificationConfig, error) {
	query := `SELECT tenant_id, slack_webhook_enabled, slack_webhook_secret_ref, slack_webhook_default_channel,
		slack_bot_enabled, slack_bot_secret_ref, slack_bot_default_channel, slack_bot_signing_secret_ref,
		email_enabled, email_provider, email_secret_ref, email_from, email_region, email_domain, email_default_mailing_list_id,
		notify_approval_expiring, notify_token_abuse, notify_policy_invalid, created_at, updated_at
		FROM rbitr.notification_config WHERE tenant_id = $1`
	row := s.db.QueryRowContext(ctx, query, tenantID)
	var config models.NotificationConfig
	var webhookRef sql.NullString
	var webhookChannel sql.NullString
	var botRef sql.NullString
	var botChannel sql.NullString
	var botSigningRef sql.NullString
	var emailProvider sql.NullString
	var emailRef sql.NullString
	var emailFrom sql.NullString
	var emailRegion sql.NullString
	var emailDomain sql.NullString
	var emailListID sql.NullString
	if err := row.Scan(
		&config.TenantID,
		&config.SlackWebhookEnabled,
		&webhookRef,
		&webhookChannel,
		&config.SlackBotEnabled,
		&botRef,
		&botChannel,
		&botSigningRef,
		&config.EmailEnabled,
		&emailProvider,
		&emailRef,
		&emailFrom,
		&emailRegion,
		&emailDomain,
		&emailListID,
		&config.NotifyApprovalExpiring,
		&config.NotifyTokenAbuse,
		&config.NotifyPolicyInvalid,
		&config.CreatedAt,
		&config.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.NotificationConfig{}, ErrNotFound
		}
		return models.NotificationConfig{}, err
	}
	if webhookRef.Valid {
		config.SlackWebhookSecretRef = webhookRef.String
	}
	if webhookChannel.Valid {
		config.SlackWebhookDefaultChannel = webhookChannel.String
	}
	if botRef.Valid {
		config.SlackBotSecretRef = botRef.String
	}
	if botChannel.Valid {
		config.SlackBotDefaultChannel = botChannel.String
	}
	if botSigningRef.Valid {
		config.SlackBotSigningSecretRef = botSigningRef.String
	}
	if emailProvider.Valid {
		config.EmailProvider = emailProvider.String
	}
	if emailRef.Valid {
		config.EmailSecretRef = emailRef.String
	}
	if emailFrom.Valid {
		config.EmailFrom = emailFrom.String
	}
	if emailRegion.Valid {
		config.EmailRegion = emailRegion.String
	}
	if emailDomain.Valid {
		config.EmailDomain = emailDomain.String
	}
	if emailListID.Valid {
		config.EmailDefaultMailingListID = emailListID.String
	}
	return config, nil
}

func (s *Store) UpsertNotificationConfig(ctx context.Context, config models.NotificationConfig) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.notification_config (
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
		updated_at = EXCLUDED.updated_at`,
		config.TenantID,
		config.SlackWebhookEnabled,
		nullableString(config.SlackWebhookSecretRef),
		nullableString(config.SlackWebhookDefaultChannel),
		config.SlackBotEnabled,
		nullableString(config.SlackBotSecretRef),
		nullableString(config.SlackBotDefaultChannel),
		nullableString(config.SlackBotSigningSecretRef),
		config.EmailEnabled,
		nullableString(config.EmailProvider),
		nullableString(config.EmailSecretRef),
		nullableString(config.EmailFrom),
		nullableString(config.EmailRegion),
		nullableString(config.EmailDomain),
		nullableString(config.EmailDefaultMailingListID),
		config.NotifyApprovalExpiring,
		config.NotifyTokenAbuse,
		config.NotifyPolicyInvalid,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	return err
}

func (s *Store) ListMailingLists(ctx context.Context, tenantID string) ([]models.MailingList, error) {
	query := `SELECT mailing_list_id, tenant_id, name, description, created_at, updated_at
		FROM rbitr.mailing_lists
		WHERE tenant_id = $1
		ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []models.MailingList
	for rows.Next() {
		var list models.MailingList
		var description sql.NullString
		if err := rows.Scan(&list.MailingListID, &list.TenantID, &list.Name, &description, &list.CreatedAt, &list.UpdatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			list.Description = description.String
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}

func (s *Store) GetMailingList(ctx context.Context, tenantID, mailingListID string) (models.MailingList, error) {
	query := `SELECT mailing_list_id, tenant_id, name, description, created_at, updated_at
		FROM rbitr.mailing_lists
		WHERE tenant_id = $1 AND mailing_list_id = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, mailingListID)
	var list models.MailingList
	var description sql.NullString
	if err := row.Scan(&list.MailingListID, &list.TenantID, &list.Name, &description, &list.CreatedAt, &list.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.MailingList{}, ErrNotFound
		}
		return models.MailingList{}, err
	}
	if description.Valid {
		list.Description = description.String
	}
	return list, nil
}

func (s *Store) ListMailingListMembers(ctx context.Context, mailingListID string) ([]models.MailingListMember, error) {
	query := `SELECT mailing_list_id, email, created_at
		FROM rbitr.mailing_list_members
		WHERE mailing_list_id = $1
		ORDER BY email`
	rows, err := s.db.QueryContext(ctx, query, mailingListID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.MailingListMember
	for rows.Next() {
		var member models.MailingListMember
		if err := rows.Scan(&member.MailingListID, &member.Email, &member.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) CreateMailingList(ctx context.Context, list models.MailingList, members []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO rbitr.mailing_lists (mailing_list_id, tenant_id, name, description, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		list.MailingListID,
		list.TenantID,
		list.Name,
		nullableString(list.Description),
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	for _, email := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rbitr.mailing_list_members (mailing_list_id, email, created_at)
			VALUES ($1,$2,$3)`,
			list.MailingListID,
			email,
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateMailingList(ctx context.Context, list models.MailingList, members []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `UPDATE rbitr.mailing_lists
		SET name = $1, description = $2, updated_at = $3
		WHERE tenant_id = $4 AND mailing_list_id = $5`,
		list.Name,
		nullableString(list.Description),
		time.Now().UTC(),
		list.TenantID,
		list.MailingListID,
	)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM rbitr.mailing_list_members WHERE mailing_list_id = $1`, list.MailingListID); err != nil {
		return err
	}
	for _, email := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rbitr.mailing_list_members (mailing_list_id, email, created_at)
			VALUES ($1,$2,$3)`,
			list.MailingListID,
			email,
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteMailingList(ctx context.Context, tenantID, mailingListID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rbitr.mailing_lists WHERE tenant_id = $1 AND mailing_list_id = $2`, tenantID, mailingListID)
	return err
}

func (s *Store) GetNotificationSuppression(ctx context.Context, dedupKey string) (models.NotificationSuppression, error) {
	query := `SELECT dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until, suppressed_count, last_payload_hash, updated_at
		FROM rbitr.notification_suppressions WHERE dedup_key = $1`
	row := s.db.QueryRowContext(ctx, query, dedupKey)
	var suppression models.NotificationSuppression
	var resourceID sql.NullString
	var lastSentAt sql.NullTime
	var suppressedUntil sql.NullTime
	var lastPayloadHash sql.NullString
	if err := row.Scan(
		&suppression.DedupKey,
		&suppression.TenantID,
		&suppression.Channel,
		&suppression.EventType,
		&resourceID,
		&suppression.Severity,
		&suppression.FirstSeenAt,
		&suppression.LastSeenAt,
		&lastSentAt,
		&suppressedUntil,
		&suppression.SuppressedCount,
		&lastPayloadHash,
		&suppression.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.NotificationSuppression{}, ErrNotFound
		}
		return models.NotificationSuppression{}, err
	}
	if resourceID.Valid {
		suppression.ResourceID = resourceID.String
	}
	if lastSentAt.Valid {
		suppression.LastSentAt = &lastSentAt.Time
	}
	if suppressedUntil.Valid {
		suppression.SuppressedUntil = &suppressedUntil.Time
	}
	if lastPayloadHash.Valid {
		suppression.LastPayloadHash = lastPayloadHash.String
	}
	return suppression, nil
}

func (s *Store) UpsertNotificationSuppression(ctx context.Context, suppression models.NotificationSuppression) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.notification_suppressions (
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
		updated_at = EXCLUDED.updated_at`,
		suppression.DedupKey,
		suppression.TenantID,
		suppression.Channel,
		suppression.EventType,
		nullableString(suppression.ResourceID),
		suppression.Severity,
		suppression.FirstSeenAt,
		suppression.LastSeenAt,
		suppression.LastSentAt,
		suppression.SuppressedUntil,
		suppression.SuppressedCount,
		nullableString(suppression.LastPayloadHash),
		time.Now().UTC(),
	)
	return err
}

func (s *Store) ListApprovalsExpiring(ctx context.Context, now time.Time, window time.Duration) ([]models.ApprovalRequest, error) {
	cutoff := now.Add(window)
	query := `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, reasons
		FROM rbitr.approval_requests
		WHERE status IN ('PENDING','APPROVED')
			AND expires_at > $1
			AND expires_at <= $2
		ORDER BY expires_at ASC`
	rows, err := s.db.QueryContext(ctx, query, now, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []models.ApprovalRequest
	for rows.Next() {
		approval, err := scanApprovalRequest(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func (s *Store) ListApprovalsExpired(ctx context.Context, now time.Time) ([]models.ApprovalRequest, error) {
	query := `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, reasons
		FROM rbitr.approval_requests
		WHERE status IN ('PENDING','APPROVED')
			AND expires_at <= $1
		ORDER BY expires_at ASC`
	rows, err := s.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []models.ApprovalRequest
	for rows.Next() {
		approval, err := scanApprovalRequest(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func (s *Store) TryAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key)
	var ok bool
	if err := row.Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (s *Store) ReleaseAdvisoryLock(ctx context.Context, key int64) error {
	row := s.db.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, key)
	var ok bool
	if err := row.Scan(&ok); err != nil {
		return err
	}
	return nil
}

func hashKey(key string) string {
	return utils.HashString(key)
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanApprovalRequest(scanner rowScanner) (models.ApprovalRequest, error) {
	var (
		decidedAt          sql.NullTime
		policyVersion      sql.NullString
		decidedBy          sql.NullString
		decisionComment    sql.NullString
		executedAt         sql.NullTime
		executedRequestID  sql.NullString
		executedDecisionID sql.NullString
		requestDecisionID  sql.NullString
		actionSummary      sql.NullString
		risk               sql.NullString
		ruleID             sql.NullString
		reasonsJSON        []byte
	)

	var approval models.ApprovalRequest
	if err := scanner.Scan(
		&approval.ApprovalRequestID,
		&approval.TenantID,
		&approval.AgentID,
		&approval.ToolID,
		&approval.ActionType,
		&approval.RequestHash,
		&approval.Status,
		&approval.ApprovalTokenHash,
		&approval.ExpiresAt,
		&approval.CreatedAt,
		&policyVersion,
		&decidedAt,
		&decidedBy,
		&decisionComment,
		&executedAt,
		&executedRequestID,
		&executedDecisionID,
		&requestDecisionID,
		&actionSummary,
		&risk,
		&ruleID,
		&reasonsJSON,
	); err != nil {
		return models.ApprovalRequest{}, err
	}

	if decidedAt.Valid {
		approval.DecidedAt = &decidedAt.Time
	}
	if policyVersion.Valid {
		approval.PolicyVersion = policyVersion.String
	}
	if decidedBy.Valid {
		approval.DecidedBy = decidedBy.String
	}
	if decisionComment.Valid {
		approval.DecisionComment = decisionComment.String
	}
	if executedAt.Valid {
		approval.ExecutedAt = &executedAt.Time
	}
	if executedRequestID.Valid {
		approval.ExecutedRequestID = executedRequestID.String
	}
	if executedDecisionID.Valid {
		approval.ExecutedDecisionID = executedDecisionID.String
	}
	if requestDecisionID.Valid {
		approval.RequestDecisionID = requestDecisionID.String
	}
	if actionSummary.Valid {
		approval.ActionSummary = actionSummary.String
	}
	if risk.Valid {
		approval.Risk = risk.String
	}
	if ruleID.Valid {
		approval.RuleID = ruleID.String
	}
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &approval.Reasons); err != nil {
			return models.ApprovalRequest{}, err
		}
	}

	return approval, nil
}
