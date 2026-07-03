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

	"github.com/gabrielleeyj/rbitr/internal/audit"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrBootstrapComplete = errors.New("bootstrap already completed")
	ErrAdminWriteLocked  = errors.New("admin writes locked")
	ErrInvalidState      = errors.New("invalid state")
	ErrDuplicate         = errors.New("duplicate")
)

const (
	bootstrapKey                 = "bootstrap_complete"
	adminWriteLockKey            = "admin_write_lock"
	defaultApprovalTTLSecondsKey = "default_approval_ttl_seconds"
	auditRetentionDaysKey        = "audit_retention_days"
	disableXTenantKeyKey         = "disable_x_tenant_key"
	featureRateLimitingKey       = "feature_rate_limiting"
	featureArgConstraintsKey     = "feature_arg_constraints"
	featureSessionTokensKey      = "feature_session_tokens"
	featureFileGovernanceKey     = "feature_file_governance"
	sessionTokenTTLSecondsKey    = "session_token_ttl_seconds"
	secretProviderAWSKey         = "secret_provider_aws"
	secretProviderGCPKey         = "secret_provider_gcp"
	secretProviderVaultKey       = "secret_provider_vault"
	secretProviderAzureKey       = "secret_provider_azure"
	ssoEnabledKey                = "sso_enabled"
	ssoIssuerKey                 = "sso_issuer"
	ssoClientIDKey               = "sso_client_id"
	ssoClientSecretRefKey        = "sso_client_secret_ref" //nolint:gosec // setting key name, not a credential
	ssoRedirectURIKey            = "sso_redirect_uri"
	ssoAllowedDomainsKey         = "sso_allowed_domains"
	ssoDefaultScopesKey          = "sso_default_scopes"
	defaultRateLimitPerMinuteKey = "default_rate_limit_per_minute"
	defaultRateLimitPerDayKey    = "default_rate_limit_per_day"
	defaultRateLimitScopeKey     = "default_rate_limit_scope"
	settingTrue                  = "true"
	settingFalse                 = "false"
	defaultRateLimitPerMinuteVal = int64(60)
	defaultRateLimitPerDayVal    = int64(10000)
	defaultRateLimitScopeVal     = "tenant_agent_tool"
	defaultPageLimit             = 50

	toolSourceAdmin           = "admin"
	whereTenantID             = "tenant_id = $1"
	rateLimitScopeTenantAgent = "tenant_agent"
	rateLimitScopeTenantTool  = "tenant_tool"
)

type StoreAPI interface {
	GetTenantByKeyHash(ctx context.Context, keyHash string) (models.Tenant, error)
	GetAdminKeyByHash(ctx context.Context, keyHash string) (models.AdminKey, error)
	ListTenants(ctx context.Context) ([]models.TenantSummary, error)
	GetTenant(ctx context.Context, tenantID string) (models.TenantSummary, error)
	GetTenantKeyHash(ctx context.Context, tenantID string) (string, error)
	GetTool(ctx context.Context, tenantID, toolID string) (models.Tool, error)
	ListTools(ctx context.Context, tenantID string, includeArchived, excludeDevSeeds bool) ([]models.Tool, error)
	InsertTool(ctx context.Context, tool *models.Tool) error
	ArchiveTool(ctx context.Context, tenantID, toolID string) error
	RestoreTool(ctx context.Context, tenantID, toolID string) error
	GetPolicy(ctx context.Context, tenantID string) (models.Policy, error)
	GetTenantConfig(ctx context.Context, tenantID string) (models.TenantConfig, error)
	GetEffectiveRateLimitConfig(ctx context.Context, tenantID string) (models.RateLimitConfig, error)
	ListPolicyVersions(ctx context.Context, tenantID string) ([]models.PolicyVersion, error)
	GetPolicyVersion(ctx context.Context, tenantID, policyVersion string) (models.PolicyVersion, error)
	CreatePolicyVersion(ctx context.Context, tenantID, policyVersion, regoModule, createdBy, notes string) error
	CreatePolicyVersionStructured(ctx context.Context, tenantID, policyVersion, regoModule string, structuredJSON []byte, createdBy, notes string) error
	PublishPolicyVersion(ctx context.Context, tenantID, policyVersion string) error
	RollbackPolicyVersion(ctx context.Context, tenantID, policyVersion string) error
	ListFallbackHitPairs(ctx context.Context, tenantID string, ruleIDs []string, since time.Time, limit int) ([]models.CoverageFallbackHit, error)
	ListUnusedActiveTools(ctx context.Context, tenantID string) ([]string, error)
	GetRiskOverride(ctx context.Context, tenantID, actionType string) (string, error)
	ListRiskOverrides(ctx context.Context, tenantID string) ([]models.RiskOverride, error)
	DeleteRiskOverride(ctx context.Context, tenantID, actionType string) error
	InsertADR(ctx context.Context, record *models.ActionDecisionRecord) error
	InsertApprovalRequest(ctx context.Context, req *models.ApprovalRequest) error
	ListApprovalRequests(ctx context.Context, tenantID, status string, limit, offset int) ([]models.ApprovalRequest, error)
	GetApprovalRequest(ctx context.Context, tenantID, approvalRequestID string) (models.ApprovalRequest, error)
	GetApprovalForExecution(ctx context.Context, tenantID, approvalRequestID string) (models.ApprovalRequest, error)
	CountPendingApprovals(ctx context.Context, tenantID string, now time.Time) (int, error)
	ApproveApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	DenyApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	RevokeApprovalRequest(ctx context.Context, tenantID, approvalRequestID, decidedBy, comment string, decidedAt time.Time) error
	ClaimApprovalExecution(ctx context.Context, tenantID, approvalRequestID, tokenHash, requestHash string, executingAt time.Time) error
	MarkApprovalExecuted(ctx context.Context, tenantID, approvalRequestID, requestID, decisionID string, executedAt time.Time) error
	MarkApprovalExecutionFailed(ctx context.Context, tenantID, approvalRequestID, errorCode string, failedAt time.Time) error
	MarkApprovalExpired(ctx context.Context, tenantID, approvalRequestID string, expiredAt time.Time) error
	ListEvidence(ctx context.Context, tenantID string, limit int) ([]models.ActionDecisionRecord, error)
	ListEvidenceFiltered(ctx context.Context, tenantID, decision, actionType, risk string, since *time.Time, limit int) ([]models.ActionDecisionRecord, error)
	UpdateTenantConfig(ctx context.Context, tenantID, name, tenantKey string) error
	UpdateToolConfig(ctx context.Context, tenantID, toolID, baseURL, authType, authValue string, credentialConfig json.RawMessage) error
	UpdateToolMetadata(ctx context.Context, tenantID, toolID, description, mcpUpstreamURL string, inputSchemaJSON []byte) error
	UpdatePolicy(ctx context.Context, tenantID, regoModule, policyVersion string) error
	UpdateRiskOverride(ctx context.Context, tenantID, actionType, actionRisk string) error
	MarkBootstrapComplete(ctx context.Context) error
	GetBootstrapComplete(ctx context.Context) (bool, error)
	SetAdminWriteLock(ctx context.Context, locked bool) error
	GetAdminWriteLock(ctx context.Context) (bool, error)
	SetDefaultApprovalTTLSeconds(ctx context.Context, seconds int) error
	GetDefaultApprovalTTLSeconds(ctx context.Context) (int, error)
	SetAuditRetentionDays(ctx context.Context, days int) error
	GetAuditRetentionDays(ctx context.Context) (int, error)
	SetDisableXTenantKey(ctx context.Context, disabled bool) error
	GetDisableXTenantKey(ctx context.Context) (bool, error)
	SetFeatureRateLimiting(ctx context.Context, enabled bool) error
	GetFeatureRateLimiting(ctx context.Context) (bool, error)
	SetFeatureArgConstraints(ctx context.Context, enabled bool) error
	GetFeatureArgConstraints(ctx context.Context) (bool, error)
	SetFeatureSessionTokens(ctx context.Context, enabled bool) error
	GetFeatureSessionTokens(ctx context.Context) (bool, error)
	SetFeatureFileGovernance(ctx context.Context, enabled bool) error
	GetFeatureFileGovernance(ctx context.Context) (bool, error)
	SetSessionTokenTTLSeconds(ctx context.Context, seconds int) error
	GetSessionTokenTTLSeconds(ctx context.Context) (int, error)
	SetSecretProviderAWS(ctx context.Context, enabled bool) error
	GetSecretProviderAWS(ctx context.Context) (bool, error)
	SetSecretProviderGCP(ctx context.Context, enabled bool) error
	GetSecretProviderGCP(ctx context.Context) (bool, error)
	SetSecretProviderVault(ctx context.Context, enabled bool) error
	GetSecretProviderVault(ctx context.Context) (bool, error)
	SetSecretProviderAzure(ctx context.Context, enabled bool) error
	GetSecretProviderAzure(ctx context.Context) (bool, error)
	SetDefaultRateLimitConfig(ctx context.Context, perMinute, perDay int64, scope string) error
	GetDefaultRateLimitConfig(ctx context.Context) (models.RateLimitConfig, error)
	SetTenantEnforcementMode(ctx context.Context, tenantID, enforcementMode string) error
	SetTenantMCPPassthroughUpstreamToolID(ctx context.Context, tenantID, toolID string) error
	IncrementRateLimitCounter(ctx context.Context, tenantID, agentID, toolID, actionType, window string, bucketStart, now time.Time, limit int64) (allowed bool, count int64, err error)
	ListAuditEvents(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string, from, to *time.Time) ([]models.AdminAuditEvent, error)
	ListAuditEventsExport(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string, from, to *time.Time) ([]models.AdminAuditEvent, error)
	ListAuditResourceTypes(ctx context.Context, tenantID string) ([]string, error)
	InsertAuditEvent(ctx context.Context, event *models.AdminAuditEvent) error
	DeleteAuditEventsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	GetNotificationConfig(ctx context.Context, tenantID string) (models.NotificationConfig, error)
	UpsertNotificationConfig(ctx context.Context, config *models.NotificationConfig) error
	ListMailingLists(ctx context.Context, tenantID string) ([]models.MailingList, error)
	GetMailingList(ctx context.Context, tenantID, mailingListID string) (models.MailingList, error)
	ListMailingListMembers(ctx context.Context, mailingListID string) ([]models.MailingListMember, error)
	CreateMailingList(ctx context.Context, list *models.MailingList, members []string) error
	UpdateMailingList(ctx context.Context, list *models.MailingList, members []string) error
	DeleteMailingList(ctx context.Context, tenantID, mailingListID string) error
	GetNotificationSuppression(ctx context.Context, dedupKey string) (models.NotificationSuppression, error)
	UpsertNotificationSuppression(ctx context.Context, suppression *models.NotificationSuppression) error
	ListNotificationSuppressions(ctx context.Context, tenantID string, limit, offset int, eventType, channel, severity string) ([]models.NotificationSuppression, error)
	ListApprovalsExpiring(ctx context.Context, now time.Time, window time.Duration) ([]models.ApprovalRequest, error)
	ListApprovalsExpired(ctx context.Context, now time.Time) ([]models.ApprovalRequest, error)
	TryAdvisoryLock(ctx context.Context, key int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, key int64) error

	// SSO config (Epic 12 Phase 5)
	SetSSOEnabled(ctx context.Context, enabled bool) error
	GetSSOEnabled(ctx context.Context) (bool, error)
	SetSSOConfig(ctx context.Context, issuer, clientID, clientSecretRef, redirectURI, allowedDomains, defaultScopes string) error
	GetSSOConfig(ctx context.Context) (SSOConfig, error)

	// Ticketing config (Epic 12 Phase 3)
	GetTicketingConfig(ctx context.Context, tenantID string) (models.TicketingConfig, error)
	UpsertTicketingConfig(ctx context.Context, config *models.TicketingConfig) error
	InsertTicketLink(ctx context.Context, link *models.TicketLink) error
	GetTicketLinkByApproval(ctx context.Context, tenantID, approvalRequestID string) (models.TicketLink, error)
	GetTicketLinkByExternalKey(ctx context.Context, provider, externalKey string) (models.TicketLink, error)
	ListTicketLinks(ctx context.Context, tenantID string, limit, offset int) ([]models.TicketLink, error)
	UpdateTicketLinkStatus(ctx context.Context, ticketLinkID, status string) error

	// License history (Epic 13)
	InsertLicenseHistory(ctx context.Context, tier string, keyVersion int, licensee, email string, expiresAt time.Time, fingerprint string) error
	GetLatestLicenseHistory(ctx context.Context) (LicenseHistoryRecord, error)
	GetEarliestTrialStartDate(ctx context.Context) (*time.Time, error)
	HasTrialLicenseBeenUsed(ctx context.Context) (bool, error)

	// Usage metering (Epic 13 Phase 2)
	IncrementUsageMeter(ctx context.Context, tenantID, period string) (int64, error)
	GetUsageMeter(ctx context.Context, tenantID, period string) (int64, error)
	ListUsageMeters(ctx context.Context, tenantID string, limit int) ([]UsageMeterRecord, error)

	// Usage dashboard (Epic 13 Phase 6)
	GetTotalUsageForPeriod(ctx context.Context, period string) (int64, error)
	ListAggregatedUsageHistory(ctx context.Context, limit int) ([]PeriodUsageSummary, error)

	// Tenant management (Epic 7)
	CreateTenant(ctx context.Context, tenantID, name string) error
	SetTenantEnabled(ctx context.Context, tenantID string, enabled bool) error

	// Tenant key lifecycle (Epic 7)
	CreateTenantKey(ctx context.Context, key *models.TenantKey) error
	ListTenantKeys(ctx context.Context, tenantID string) ([]models.TenantKey, error)
	RevokeTenantKey(ctx context.Context, tenantID, keyID string, revokedAt time.Time) error

	// Provisioning limits (Epic 13 Phase 3)
	CountTenants(ctx context.Context) (int, error)
	CountActiveKeysByTenant(ctx context.Context, tenantID string) (int, error)
}

// LicenseHistoryRecord represents a license activation event.
type LicenseHistoryRecord struct {
	ID          string    `json:"id"`
	Tier        string    `json:"tier"`
	KeyVersion  int       `json:"key_version"`
	Licensee    string    `json:"licensee"`
	Email       string    `json:"email"`
	ExpiresAt   time.Time `json:"expires_at"`
	ActivatedAt time.Time `json:"activated_at"`
	Fingerprint string    `json:"fingerprint"`
}

// UsageMeterRecord represents a tenant's action count for a billing period.
type UsageMeterRecord struct {
	TenantID    string    `json:"tenant_id"`
	Period      string    `json:"period"`
	ActionCount int64     `json:"action_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PeriodUsageSummary holds aggregated action counts across all tenants for a billing period.
type PeriodUsageSummary struct {
	Period      string `json:"period"`
	ActionCount int64  `json:"action_count"`
}

// SSOConfig holds SSO/OIDC configuration persisted in system_settings.
type SSOConfig struct {
	Enabled         bool
	Issuer          string
	ClientID        string
	ClientSecretRef string
	RedirectURI     string
	AllowedDomains  string
	DefaultScopes   string
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
	query := `SELECT t.tenant_id, t.name, t.enabled
		FROM rbitr.tenant_keys tk
		JOIN rbitr.tenants t ON t.tenant_id = tk.tenant_id
		WHERE tk.key_hash = $1
		  AND tk.revoked_at IS NULL
		  AND t.enabled = true
		  AND t.deleted_at IS NULL`
	row := s.db.QueryRowContext(ctx, query, keyHash)
	if err := row.Scan(&tenant.TenantID, &tenant.Name, &tenant.Enabled); err != nil {
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
		WHERE t.deleted_at IS NULL
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
		WHERE t.tenant_id = $1
		  AND t.deleted_at IS NULL`
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
	var mcpUpstreamURL, description, openapiSpecURL, openapiOperationID sql.NullString
	var archivedAt sql.NullTime
	var inputSchemaJSON, credentialConfig []byte
	query := `SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json, archived_at, source, openapi_spec_url, openapi_operation_id, credential_config FROM rbitr.tools WHERE tenant_id = $1 AND tool_id = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, toolID)
	if err := row.Scan(&tool.ToolID, &tool.TenantID, &tool.BaseURL, &tool.AuthType, &tool.AuthValue, &tool.Transport, &mcpUpstreamURL, &description, &inputSchemaJSON, &archivedAt, &tool.Source, &openapiSpecURL, &openapiOperationID, &credentialConfig); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Tool{}, ErrNotFound
		}
		return models.Tool{}, err
	}
	if mcpUpstreamURL.Valid {
		tool.MCPUpstreamURL = mcpUpstreamURL.String
	}
	if description.Valid {
		tool.Description = description.String
	}
	if len(inputSchemaJSON) > 0 {
		tool.InputSchemaJSON = json.RawMessage(inputSchemaJSON)
	}
	if archivedAt.Valid {
		tool.ArchivedAt = &archivedAt.Time
	}
	if openapiSpecURL.Valid {
		tool.OpenAPISpecURL = openapiSpecURL.String
	}
	if openapiOperationID.Valid {
		tool.OpenAPIOperationID = openapiOperationID.String
	}
	if len(credentialConfig) > 0 {
		tool.CredentialConfig = json.RawMessage(credentialConfig)
	}
	return tool, nil
}

func (s *Store) ListTools(ctx context.Context, tenantID string, includeArchived, excludeDevSeeds bool) ([]models.Tool, error) {
	query := `SELECT tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json, archived_at, source, openapi_spec_url, openapi_operation_id, credential_config FROM rbitr.tools WHERE tenant_id = $1`
	if !includeArchived {
		query += ` AND archived_at IS NULL`
	}
	if excludeDevSeeds {
		query += ` AND source != 'dev_seed'`
	}
	query += ` ORDER BY tool_id`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []models.Tool
	for rows.Next() {
		var tool models.Tool
		var mcpUpstreamURL, description, openapiSpecURL, openapiOperationID sql.NullString
		var archivedAt sql.NullTime
		var inputSchemaJSON, credentialConfig []byte
		if err := rows.Scan(&tool.ToolID, &tool.TenantID, &tool.BaseURL, &tool.AuthType, &tool.AuthValue, &tool.Transport, &mcpUpstreamURL, &description, &inputSchemaJSON, &archivedAt, &tool.Source, &openapiSpecURL, &openapiOperationID, &credentialConfig); err != nil {
			return nil, err
		}
		if mcpUpstreamURL.Valid {
			tool.MCPUpstreamURL = mcpUpstreamURL.String
		}
		if description.Valid {
			tool.Description = description.String
		}
		if len(inputSchemaJSON) > 0 {
			tool.InputSchemaJSON = json.RawMessage(inputSchemaJSON)
		}
		if archivedAt.Valid {
			tool.ArchivedAt = &archivedAt.Time
		}
		if openapiSpecURL.Valid {
			tool.OpenAPISpecURL = openapiSpecURL.String
		}
		if openapiOperationID.Valid {
			tool.OpenAPIOperationID = openapiOperationID.String
		}
		if len(credentialConfig) > 0 {
			tool.CredentialConfig = json.RawMessage(credentialConfig)
		}
		tools = append(tools, tool)
	}
	return tools, rows.Err()
}

func (s *Store) InsertTool(ctx context.Context, tool *models.Tool) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	source := tool.Source
	if source == "" {
		source = toolSourceAdmin
	}
	var credCfg []byte
	if len(tool.CredentialConfig) > 0 {
		credCfg = []byte(tool.CredentialConfig)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rbitr.tools (tool_id, tenant_id, base_url, auth_type, auth_value, transport, mcp_upstream_url, description, input_schema_json, source, openapi_spec_url, openapi_operation_id, credential_config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())`,
		tool.ToolID, tool.TenantID, tool.BaseURL, tool.AuthType, tool.AuthValue, tool.Transport,
		sql.NullString{String: tool.MCPUpstreamURL, Valid: tool.MCPUpstreamURL != ""},
		sql.NullString{String: tool.Description, Valid: tool.Description != ""},
		[]byte(tool.InputSchemaJSON),
		source,
		sql.NullString{String: tool.OpenAPISpecURL, Valid: tool.OpenAPISpecURL != ""},
		sql.NullString{String: tool.OpenAPIOperationID, Valid: tool.OpenAPIOperationID != ""},
		credCfg,
	)
	if err != nil {
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
			return ErrDuplicate
		}
		return err
	}

	return s.bumpTenantConfigVersion(ctx, tool.TenantID)
}

func (s *Store) ArchiveTool(ctx context.Context, tenantID, toolID string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE rbitr.tools SET archived_at = now()
		WHERE tenant_id = $1 AND tool_id = $2 AND archived_at IS NULL`,
		tenantID, toolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	return s.bumpTenantConfigVersion(ctx, tenantID)
}

func (s *Store) RestoreTool(ctx context.Context, tenantID, toolID string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE rbitr.tools SET archived_at = NULL
		WHERE tenant_id = $1 AND tool_id = $2 AND archived_at IS NOT NULL`,
		tenantID, toolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	return s.bumpTenantConfigVersion(ctx, tenantID)
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
	row := s.db.QueryRowContext(ctx, `SELECT tenant_id, active_policy_version, created_at, updated_at, enforcement_mode, mcp_passthrough_upstream_tool_id, version, trial_started_at FROM rbitr.tenant_config WHERE tenant_id = $1`, tenantID)
	var config models.TenantConfig
	var mcpUpstreamToolID sql.NullString
	var trialStartedAt sql.NullTime
	if err := row.Scan(&config.TenantID, &config.ActivePolicyVersion, &config.CreatedAt, &config.UpdatedAt, &config.EnforcementMode, &mcpUpstreamToolID, &config.Version, &trialStartedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TenantConfig{}, ErrNotFound
		}
		return models.TenantConfig{}, err
	}
	if strings.TrimSpace(config.EnforcementMode) == "" {
		config.EnforcementMode = "enforce"
	}
	if mcpUpstreamToolID.Valid {
		config.MCPPassthroughUpstreamToolID = mcpUpstreamToolID.String
	}
	if trialStartedAt.Valid {
		config.TrialStartedAt = &trialStartedAt.Time
	}
	return config, nil
}

func (s *Store) GetEffectiveRateLimitConfig(ctx context.Context, tenantID string) (models.RateLimitConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
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
		WHERE t.tenant_id = $1
		  AND t.deleted_at IS NULL`,
		tenantID,
		defaultRateLimitPerMinuteKey,
		defaultRateLimitPerDayKey,
		defaultRateLimitScopeKey,
	)

	var (
		tenantMinute sql.NullInt64
		tenantDay    sql.NullInt64
		tenantScope  sql.NullString
		systemMinute sql.NullString
		systemDay    sql.NullString
		systemScope  sql.NullString
	)
	if err := row.Scan(
		&tenantMinute,
		&tenantDay,
		&tenantScope,
		&systemMinute,
		&systemDay,
		&systemScope,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.RateLimitConfig{}, ErrNotFound
		}
		return models.RateLimitConfig{}, err
	}

	config := models.RateLimitConfig{
		PerMinute: defaultRateLimitPerMinuteVal,
		PerDay:    defaultRateLimitPerDayVal,
		Scope:     defaultRateLimitScopeVal,
	}

	if parsed, ok := parseOptionalPositiveInt64(systemMinute); ok {
		config.PerMinute = parsed
	}
	if parsed, ok := parseOptionalPositiveInt64(systemDay); ok {
		config.PerDay = parsed
	}
	if parsed, ok := parseOptionalScope(systemScope); ok {
		config.Scope = parsed
	}

	if tenantMinute.Valid && tenantMinute.Int64 > 0 {
		config.PerMinute = tenantMinute.Int64
	}
	if tenantDay.Valid && tenantDay.Int64 > 0 {
		config.PerDay = tenantDay.Int64
	}
	if parsed, ok := parseOptionalScope(tenantScope); ok {
		config.Scope = parsed
	}

	return config, nil
}

// Policy authoring modes recorded on policy_versions.authoring_mode.
const (
	AuthoringModeRego       = "rego"
	AuthoringModeStructured = "structured"
)

const policyVersionColumns = `tenant_id, policy_version, rego_module, created_at, created_by, notes, structured_json, authoring_mode`

// scanPolicyVersion scans a policy_versions row selected with policyVersionColumns.
func scanPolicyVersion(scan func(dest ...any) error) (models.PolicyVersion, error) {
	var version models.PolicyVersion
	var createdBy, notes, authoringMode sql.NullString
	var structuredJSON []byte
	if err := scan(&version.TenantID, &version.PolicyVersion, &version.RegoModule, &version.CreatedAt,
		&createdBy, &notes, &structuredJSON, &authoringMode); err != nil {
		return models.PolicyVersion{}, err
	}
	if createdBy.Valid {
		version.CreatedBy = createdBy.String
	}
	if notes.Valid {
		version.Notes = notes.String
	}
	if len(structuredJSON) > 0 {
		version.StructuredJSON = append(json.RawMessage(nil), structuredJSON...)
	}
	version.AuthoringMode = AuthoringModeRego
	if authoringMode.Valid && authoringMode.String != "" {
		version.AuthoringMode = authoringMode.String
	}
	return version, nil
}

func (s *Store) ListPolicyVersions(ctx context.Context, tenantID string) ([]models.PolicyVersion, error) {
	query := `SELECT ` + policyVersionColumns + `
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
		version, err := scanPolicyVersion(rows.Scan)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) GetPolicyVersion(ctx context.Context, tenantID, policyVersion string) (models.PolicyVersion, error) {
	query := `SELECT ` + policyVersionColumns + `
		FROM rbitr.policy_versions
		WHERE tenant_id = $1 AND policy_version = $2`
	row := s.db.QueryRowContext(ctx, query, tenantID, policyVersion)
	version, err := scanPolicyVersion(row.Scan)
	if err != nil {
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes, authoring_mode)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, policyVersion, regoModule, time.Now().UTC(), createdBy, notes, AuthoringModeRego)
	return err
}

// CreatePolicyVersionStructured persists a policy version authored via the
// structured builder, storing both the compiled Rego and the structured JSON so
// the version can be re-edited in the UI. The enforcement path still reads only
// rego_module, so this is fully compatible with hand-written policies.
func (s *Store) CreatePolicyVersionStructured(ctx context.Context, tenantID, policyVersion, regoModule string, structuredJSON []byte, createdBy, notes string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes, structured_json, authoring_mode)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenantID, policyVersion, regoModule, time.Now().UTC(), createdBy, notes, structuredJSON, AuthoringModeStructured)
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

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at, version, trial_started_at)
		VALUES ($1, $2, $3, $4, 1, $5)
		ON CONFLICT (tenant_id) DO UPDATE
		SET active_policy_version = $2, updated_at = $4, version = rbitr.tenant_config.version + 1`,
		tenantID, policyVersion, now, now, now)
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
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rbitr.action_risk_overrides WHERE tenant_id = $1 AND action_type = $2`, tenantID, actionType); err != nil {
		return err
	}
	return s.bumpTenantConfigVersion(ctx, tenantID)
}

func (s *Store) InsertADR(ctx context.Context, record *models.ActionDecisionRecord) error {
	if record == nil {
		return errors.New("action decision record required")
	}

	// Deduplicate ADR inserts for approval executions: if an ADR already
	// exists for the same approval_request_id, skip the insert to prevent
	// duplicate records on retry within the execution retry window.
	if record.ApprovalRequestID != "" {
		var exists bool
		err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM rbitr.action_decisions WHERE tenant_id = $1 AND approval_request_id = $2)`,
			record.TenantID, record.ApprovalRequestID,
		).Scan(&exists)
		if err == nil && exists {
			return nil
		}
	}

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
		response_hash, approval_request_id, source_decision_id, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`
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
		nullableString(record.SourceDecisionID),
		record.CreatedAt,
	)
	return err
}

func (s *Store) InsertApprovalRequest(ctx context.Context, req *models.ApprovalRequest) error {
	if req == nil {
		return errors.New("approval request required")
	}

	query := `INSERT INTO rbitr.approval_requests (
		approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash,
		status, approval_token_hash, expires_at, created_at, policy_version,
		decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id,
		request_decision_id, action_summary, risk, rule_id, request_context, reasons
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`
	var requestContextJSON []byte
	if len(req.RequestContext) > 0 {
		var err error
		requestContextJSON, err = json.Marshal(req.RequestContext)
		if err != nil {
			return err
		}
	}
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
		requestContextJSON,
		reasonsJSON,
	)
	return err
}

func (s *Store) ListApprovalRequests(ctx context.Context, tenantID, status string, limit, offset int) ([]models.ApprovalRequest, error) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	clauses := []string{whereTenantID}

	if status != "" {
		args = append(args, status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}

	limitPos := len(args) + 1
	offsetPos := len(args) + 2 //nolint:mnd // ignore arg pos

	args = append(args, limit, offset)

	var b strings.Builder
	b.WriteString(`SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
	approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
	executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
	risk, rule_id, request_context, reasons
	FROM rbitr.approval_requests
	WHERE `)
	b.WriteString(strings.Join(clauses, " AND "))
	b.WriteString(" ORDER BY created_at DESC LIMIT $")
	b.WriteString(strconv.Itoa(limitPos))
	b.WriteString(" OFFSET $")
	b.WriteString(strconv.Itoa(offsetPos))

	query := b.String()

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
		risk, rule_id, request_context, reasons
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

func (s *Store) GetApprovalForExecution(ctx context.Context, tenantID, approvalRequestID string) (models.ApprovalRequest, error) {
	row := s.db.QueryRowContext(ctx, `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, action_summary, risk, rule_id, request_context, reasons,
		executing_at, execution_id, failed_at, last_error_code
		FROM rbitr.approval_requests
		WHERE tenant_id = $1 AND approval_request_id = $2`, tenantID, approvalRequestID)

	var (
		policyVersion sql.NullString
		actionSummary sql.NullString
		risk          sql.NullString
		ruleID        sql.NullString
		requestJSON   []byte
		reasonsJSON   []byte
		executingAt   sql.NullTime
		executionID   sql.NullString
		failedAt      sql.NullTime
		lastErrorCode sql.NullString
	)

	var approval models.ApprovalRequest
	if err := row.Scan(
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
		&actionSummary,
		&risk,
		&ruleID,
		&requestJSON,
		&reasonsJSON,
		&executingAt,
		&executionID,
		&failedAt,
		&lastErrorCode,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ApprovalRequest{}, ErrNotFound
		}
		return models.ApprovalRequest{}, err
	}

	if policyVersion.Valid {
		approval.PolicyVersion = policyVersion.String
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
	if len(requestJSON) > 0 {
		if err := json.Unmarshal(requestJSON, &approval.RequestContext); err != nil {
			return models.ApprovalRequest{}, err
		}
	}
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &approval.Reasons); err != nil {
			return models.ApprovalRequest{}, err
		}
	}
	if executingAt.Valid {
		approval.ExecutingAt = &executingAt.Time
	}
	if executionID.Valid {
		approval.ExecutionID = executionID.String
	}
	if failedAt.Valid {
		approval.FailedAt = &failedAt.Time
	}
	if lastErrorCode.Valid {
		approval.LastErrorCode = lastErrorCode.String
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

func (s *Store) ClaimApprovalExecution(ctx context.Context, tenantID, approvalRequestID, tokenHash, requestHash string, executingAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = 'EXECUTING', executing_at = $1, execution_id = COALESCE(execution_id, approval_request_id), last_error_code = NULL
		WHERE tenant_id = $2
			AND approval_request_id = $3
			AND status = 'APPROVED'
			AND expires_at > $4
			AND approval_token_hash = $5
			AND request_hash = $6`,
		executingAt, tenantID, approvalRequestID, executingAt, tokenHash, requestHash)
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

func (s *Store) MarkApprovalExecuted(ctx context.Context, tenantID, approvalRequestID, requestID, decisionID string, executedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = 'EXECUTED', executed_at = $1, executed_request_id = $2, executed_decision_id = $3, last_error_code = NULL
		WHERE tenant_id = $4 AND approval_request_id = $5 AND status = 'EXECUTING' AND executed_at IS NULL`,
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

func (s *Store) MarkApprovalExecutionFailed(ctx context.Context, tenantID, approvalRequestID, errorCode string, failedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.approval_requests
		SET status = 'FAILED', failed_at = $1, last_error_code = $2
		WHERE tenant_id = $3 AND approval_request_id = $4 AND status = 'EXECUTING'`,
		failedAt, nullableString(errorCode), tenantID, approvalRequestID)
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
	clauses := []string{whereTenantID}

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
	//nolint:gosec // #nosec G201 - dynamic fragments are generated from fixed query clauses with positional placeholders.
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

func (s *Store) SetTenantEnforcementMode(ctx context.Context, tenantID, enforcementMode string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.tenant_config
		SET enforcement_mode = $1, updated_at = $2
		WHERE tenant_id = $3`, enforcementMode, time.Now().UTC(), tenantID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetTenantMCPPassthroughUpstreamToolID(ctx context.Context, tenantID, toolID string) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.tenant_config
		SET mcp_passthrough_upstream_tool_id = NULLIF($1, ''), updated_at = $2
		WHERE tenant_id = $3`, toolID, time.Now().UTC(), tenantID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateToolConfig(ctx context.Context, tenantID, toolID, baseURL, authType, authValue string, credentialConfig json.RawMessage) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	var credCfg []byte
	if len(credentialConfig) > 0 {
		credCfg = []byte(credentialConfig)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rbitr.tools SET base_url = $1, auth_type = $2, auth_value = $3, credential_config = $4 WHERE tenant_id = $5 AND tool_id = $6`,
		baseURL, authType, authValue, credCfg, tenantID, toolID); err != nil {
		return err
	}

	return s.bumpTenantConfigVersion(ctx, tenantID)
}

func (s *Store) UpdateToolMetadata(ctx context.Context, tenantID, toolID, description, mcpUpstreamURL string, inputSchemaJSON []byte) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE rbitr.tools
		SET description = $1, mcp_upstream_url = $2, input_schema_json = $3
		WHERE tenant_id = $4 AND tool_id = $5`,
		description, mcpUpstreamURL, inputSchemaJSON, tenantID, toolID); err != nil {
		return err
	}

	return s.bumpTenantConfigVersion(ctx, tenantID)
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

	if _, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.action_risk_overrides (tenant_id, action_type, action_risk, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, action_type) DO UPDATE SET action_risk = $3, updated_at = $4`,
		tenantID, actionType, actionRisk, time.Now().UTC()); err != nil {
		return err
	}
	return s.bumpTenantConfigVersion(ctx, tenantID)
}

func (s *Store) bumpTenantConfigVersion(ctx context.Context, tenantID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rbitr.tenant_config
		SET version = version + 1, updated_at = $2
		WHERE tenant_id = $1`, tenantID, time.Now().UTC())
	if err != nil {
		return err
	}
	_, _ = result.RowsAffected()
	return nil
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

func (s *Store) SetAuditRetentionDays(ctx context.Context, days int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		auditRetentionDaysKey,
		strconv.Itoa(days),
		time.Now().UTC(),
	)
	return err
}

func (s *Store) GetAuditRetentionDays(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, auditRetentionDaysKey)
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

func (s *Store) SetDisableXTenantKey(ctx context.Context, disabled bool) error {
	return s.setSystemSettingBool(ctx, disableXTenantKeyKey, disabled)
}

func (s *Store) GetDisableXTenantKey(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, disableXTenantKeyKey)
}

func (s *Store) SetFeatureRateLimiting(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, featureRateLimitingKey, enabled)
}

func (s *Store) GetFeatureRateLimiting(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, featureRateLimitingKey)
}

func (s *Store) SetFeatureArgConstraints(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, featureArgConstraintsKey, enabled)
}

func (s *Store) GetFeatureArgConstraints(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, featureArgConstraintsKey)
}

func (s *Store) SetFeatureSessionTokens(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, featureSessionTokensKey, enabled)
}

func (s *Store) GetFeatureSessionTokens(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, featureSessionTokensKey)
}

func (s *Store) SetFeatureFileGovernance(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, featureFileGovernanceKey, enabled)
}

func (s *Store) GetFeatureFileGovernance(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, featureFileGovernanceKey)
}

func (s *Store) SetSecretProviderAWS(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, secretProviderAWSKey, enabled)
}

func (s *Store) GetSecretProviderAWS(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, secretProviderAWSKey)
}

func (s *Store) SetSecretProviderGCP(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, secretProviderGCPKey, enabled)
}

func (s *Store) GetSecretProviderGCP(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, secretProviderGCPKey)
}

func (s *Store) SetSecretProviderVault(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, secretProviderVaultKey, enabled)
}

func (s *Store) GetSecretProviderVault(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, secretProviderVaultKey)
}

func (s *Store) SetSecretProviderAzure(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, secretProviderAzureKey, enabled)
}

func (s *Store) GetSecretProviderAzure(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, secretProviderAzureKey)
}

func (s *Store) SetSSOEnabled(ctx context.Context, enabled bool) error {
	return s.setSystemSettingBool(ctx, ssoEnabledKey, enabled)
}

func (s *Store) GetSSOEnabled(ctx context.Context) (bool, error) {
	return s.getSystemSettingBool(ctx, ssoEnabledKey)
}

func (s *Store) SetSSOConfig(ctx context.Context, issuer, clientID, clientSecretRef, redirectURI, allowedDomains, defaultScopes string) error {
	now := time.Now().UTC()
	upsertQuery := `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`

	pairs := []struct{ key, value string }{
		{ssoIssuerKey, issuer},
		{ssoClientIDKey, clientID},
		{ssoClientSecretRefKey, clientSecretRef},
		{ssoRedirectURIKey, redirectURI},
		{ssoAllowedDomainsKey, allowedDomains},
		{ssoDefaultScopesKey, defaultScopes},
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback on commit is a no-op

	for _, p := range pairs {
		if _, err := tx.ExecContext(ctx, upsertQuery, p.key, p.value, now); err != nil {
			return fmt.Errorf("set %s: %w", p.key, err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetSSOConfig(ctx context.Context) (SSOConfig, error) {
	keys := []string{
		ssoEnabledKey, ssoIssuerKey, ssoClientIDKey, ssoClientSecretRefKey,
		ssoRedirectURIKey, ssoAllowedDomainsKey, ssoDefaultScopesKey,
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM rbitr.system_settings WHERE key = ANY($1)`,
		StringArray(keys),
	)
	if err != nil {
		return SSOConfig{}, err
	}
	defer rows.Close()

	vals := make(map[string]string, len(keys))
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return SSOConfig{}, err
		}
		vals[k] = v
	}
	if err := rows.Err(); err != nil {
		return SSOConfig{}, err
	}

	return SSOConfig{
		Enabled:         vals[ssoEnabledKey] == settingTrue,
		Issuer:          vals[ssoIssuerKey],
		ClientID:        vals[ssoClientIDKey],
		ClientSecretRef: vals[ssoClientSecretRefKey],
		RedirectURI:     vals[ssoRedirectURIKey],
		AllowedDomains:  vals[ssoAllowedDomainsKey],
		DefaultScopes:   vals[ssoDefaultScopesKey],
	}, nil
}

func (s *Store) SetSessionTokenTTLSeconds(ctx context.Context, seconds int) error {
	if seconds <= 0 {
		return errors.New("session_token_ttl_seconds must be > 0")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		sessionTokenTTLSecondsKey,
		strconv.Itoa(seconds),
		time.Now().UTC(),
	)
	return err
}

func (s *Store) GetSessionTokenTTLSeconds(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, sessionTokenTTLSecondsKey)
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

func (s *Store) SetDefaultRateLimitConfig(ctx context.Context, perMinute, perDay int64, scope string) error {
	if perMinute <= 0 || perDay <= 0 {
		return errors.New("per_minute and per_day must be > 0")
	}
	if scope = strings.TrimSpace(scope); !isRateLimitScope(scope) {
		return errors.New("invalid rate limit scope")
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	upsert := `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`

	if _, err = tx.ExecContext(ctx, upsert, defaultRateLimitPerMinuteKey, strconv.FormatInt(perMinute, 10), now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, upsert, defaultRateLimitPerDayKey, strconv.FormatInt(perDay, 10), now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, upsert, defaultRateLimitScopeKey, scope, now); err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

func (s *Store) GetDefaultRateLimitConfig(ctx context.Context) (models.RateLimitConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
			s_min.value,
			s_day.value,
			s_scope.value
		FROM (SELECT 1) seed
		LEFT JOIN rbitr.system_settings s_min ON s_min.key = $1
		LEFT JOIN rbitr.system_settings s_day ON s_day.key = $2
		LEFT JOIN rbitr.system_settings s_scope ON s_scope.key = $3`,
		defaultRateLimitPerMinuteKey,
		defaultRateLimitPerDayKey,
		defaultRateLimitScopeKey,
	)

	var (
		minute sql.NullString
		day    sql.NullString
		scope  sql.NullString
	)
	if err := row.Scan(&minute, &day, &scope); err != nil {
		return models.RateLimitConfig{}, err
	}

	config := models.RateLimitConfig{
		PerMinute: defaultRateLimitPerMinuteVal,
		PerDay:    defaultRateLimitPerDayVal,
		Scope:     defaultRateLimitScopeVal,
	}
	if parsed, ok := parseOptionalPositiveInt64(minute); ok {
		config.PerMinute = parsed
	}
	if parsed, ok := parseOptionalPositiveInt64(day); ok {
		config.PerDay = parsed
	}
	if parsed, ok := parseOptionalScope(scope); ok {
		config.Scope = parsed
	}

	return config, nil
}

func (s *Store) setSystemSettingBool(ctx context.Context, key string, enabled bool) error {
	value := settingFalse
	if enabled {
		value = settingTrue
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.system_settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		key,
		value,
		time.Now().UTC(),
	)
	return err
}

func (s *Store) getSystemSettingBool(ctx context.Context, key string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, key)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	return value == settingTrue, nil
}

func (s *Store) IncrementRateLimitCounter(
	ctx context.Context,
	tenantID, agentID, toolID, actionType, window string,
	bucketStart, now time.Time,
	limit int64,
) (allowed bool, count int64, err error) {
	if limit <= 0 {
		return true, 0, nil
	}

	row := s.db.QueryRowContext(ctx, `INSERT INTO rbitr.rate_limit_counters (
			tenant_id, agent_id, tool_id, action_type, window, bucket_start, count, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7)
		ON CONFLICT (tenant_id, agent_id, tool_id, action_type, window, bucket_start)
		DO UPDATE SET
			count = rbitr.rate_limit_counters.count + 1,
			updated_at = EXCLUDED.updated_at
		WHERE rbitr.rate_limit_counters.count < $8
		RETURNING count`,
		tenantID,
		agentID,
		toolID,
		actionType,
		window,
		bucketStart,
		now,
		limit,
	)

	if err = row.Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, 0, nil
		}
		return false, 0, err
	}
	return true, count, nil
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

func (s *Store) ListAuditEvents(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string, from, to *time.Time) ([]models.AdminAuditEvent, error) {
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
	if from != nil {
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *from)
	}
	if to != nil {
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)+1))
		args = append(args, *to)
	}
	args = append(args, limit, offset)
	//nolint:gosec // #nosec G201 -- dynamic fragments are generated from fixed query clauses with positional placeholders.
	query := fmt.Sprintf(`SELECT audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
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
		var streamID sql.NullString
		var eventHash sql.NullString
		var prevHash sql.NullString
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
			&streamID,
			&eventHash,
			&prevHash,
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
		if streamID.Valid {
			event.StreamID = streamID.String
		}
		if eventHash.Valid {
			event.EventHash = eventHash.String
		}
		if prevHash.Valid {
			event.PrevHash = prevHash.String
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

func (s *Store) ListAuditEventsExport(ctx context.Context, tenantID string, limit, offset int, action, resourceType, actorID string, from, to *time.Time) ([]models.AdminAuditEvent, error) {
	if limit < 0 {
		limit = 0
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
	if from != nil {
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)+1))
		args = append(args, *from)
	}
	if to != nil {
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)+1))
		args = append(args, *to)
	}
	baseQuery := fmt.Sprintf(`SELECT audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
		FROM rbitr.admin_audit_events
		WHERE %s
		ORDER BY created_at DESC`, strings.Join(clauses, " AND "))
	query := baseQuery
	if limit > 0 {
		args = append(args, limit, offset)
		query = fmt.Sprintf(`%s LIMIT $%d OFFSET $%d`, baseQuery, len(args)-1, len(args))
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.AdminAuditEvent
	for rows.Next() {
		var event models.AdminAuditEvent
		var tenantIDValue sql.NullString
		var streamID sql.NullString
		var eventHash sql.NullString
		var prevHash sql.NullString
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
			&streamID,
			&eventHash,
			&prevHash,
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
		if streamID.Valid {
			event.StreamID = streamID.String
		}
		if eventHash.Valid {
			event.EventHash = eventHash.String
		}
		if prevHash.Valid {
			event.PrevHash = prevHash.String
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

func (s *Store) ListAuditResourceTypes(ctx context.Context, tenantID string) ([]string, error) {
	if tenantID == "" {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT resource_type
		FROM rbitr.admin_audit_events
		WHERE tenant_id = $1
		ORDER BY resource_type`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *Store) DeleteAuditEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM rbitr.admin_audit_events WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (s *Store) InsertAuditEvent(ctx context.Context, event *models.AdminAuditEvent) error {
	if event == nil {
		return errors.New("audit event required")
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.StreamID == "" {
		if event.TenantID != "" {
			event.StreamID = event.TenantID
		} else {
			event.StreamID = "global"
		}
	}
	prevHash, err := s.lastAuditHash(ctx, event.StreamID)
	if err != nil {
		return err
	}
	payload := audit.BuildHashPayload(event, event.StreamID)
	eventHash, err := audit.ComputeEventHash(prevHash, &payload)
	if err != nil {
		return err
	}
	event.EventHash = eventHash
	event.PrevHash = prevHash

	tenantID := sql.NullString{String: event.TenantID, Valid: event.TenantID != ""}
	streamID := sql.NullString{String: event.StreamID, Valid: event.StreamID != ""}
	eventHashValue := sql.NullString{String: event.EventHash, Valid: event.EventHash != ""}
	prevHashValue := sql.NullString{String: event.PrevHash, Valid: event.PrevHash != ""}
	actorID := sql.NullString{String: event.ActorID, Valid: event.ActorID != ""}
	actorDisplay := sql.NullString{String: event.ActorDisplay, Valid: event.ActorDisplay != ""}
	resourceID := sql.NullString{String: event.ResourceID, Valid: event.ResourceID != ""}
	requestID := sql.NullString{String: event.RequestID, Valid: event.RequestID != ""}
	ip := sql.NullString{String: event.IP, Valid: event.IP != ""}
	userAgent := sql.NullString{String: event.UserAgent, Valid: event.UserAgent != ""}
	_, err = s.db.ExecContext(ctx, `INSERT INTO rbitr.admin_audit_events (
		audit_event_id, tenant_id, stream_id, event_hash, prev_hash,
		actor_type, actor_id, actor_display, action, resource_type, resource_id,
		before, after, request_id, ip, user_agent, created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		event.AuditEventID,
		tenantID,
		streamID,
		eventHashValue,
		prevHashValue,
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

func (s *Store) lastAuditHash(ctx context.Context, streamID string) (string, error) {
	row := s.db.QueryRowContext(ctx, `SELECT event_hash
		FROM rbitr.admin_audit_events
		WHERE stream_id = $1 AND event_hash IS NOT NULL
		ORDER BY created_at DESC, audit_event_id DESC
		LIMIT 1`, streamID)
	var value sql.NullString
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value.String, nil
}

func (s *Store) GetNotificationConfig(ctx context.Context, tenantID string) (models.NotificationConfig, error) {
	query := `SELECT tenant_id, slack_webhook_enabled, slack_webhook_secret_ref, slack_webhook_default_channel,
		slack_bot_enabled, slack_bot_secret_ref, slack_bot_default_channel, slack_bot_signing_secret_ref,
		email_enabled, email_provider, email_secret_ref, email_from, email_region, email_domain, email_default_mailing_list_id,
		telegram_enabled, telegram_secret_ref, telegram_chat_id,
		whatsapp_enabled, whatsapp_secret_ref, whatsapp_phone_number_id, whatsapp_default_recipient,
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
	var telegramRef sql.NullString
	var telegramChatID sql.NullString
	var whatsappRef sql.NullString
	var whatsappPhoneID sql.NullString
	var whatsappRecipient sql.NullString
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
		&config.TelegramEnabled,
		&telegramRef,
		&telegramChatID,
		&config.WhatsAppEnabled,
		&whatsappRef,
		&whatsappPhoneID,
		&whatsappRecipient,
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
	if telegramRef.Valid {
		config.TelegramSecretRef = telegramRef.String
	}
	if telegramChatID.Valid {
		config.TelegramChatID = telegramChatID.String
	}
	if whatsappRef.Valid {
		config.WhatsAppSecretRef = whatsappRef.String
	}
	if whatsappPhoneID.Valid {
		config.WhatsAppPhoneNumberID = whatsappPhoneID.String
	}
	if whatsappRecipient.Valid {
		config.WhatsAppDefaultRecipient = whatsappRecipient.String
	}
	return config, nil
}

func (s *Store) UpsertNotificationConfig(ctx context.Context, config *models.NotificationConfig) error {
	if config == nil {
		return errors.New("notification config required")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO rbitr.notification_config (
		tenant_id, slack_webhook_enabled, slack_webhook_secret_ref, slack_webhook_default_channel,
		slack_bot_enabled, slack_bot_secret_ref, slack_bot_default_channel, slack_bot_signing_secret_ref,
		email_enabled, email_provider, email_secret_ref, email_from, email_region, email_domain, email_default_mailing_list_id,
		telegram_enabled, telegram_secret_ref, telegram_chat_id,
		whatsapp_enabled, whatsapp_secret_ref, whatsapp_phone_number_id, whatsapp_default_recipient,
		notify_approval_expiring, notify_token_abuse, notify_policy_invalid, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
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
		telegram_enabled = EXCLUDED.telegram_enabled,
		telegram_secret_ref = EXCLUDED.telegram_secret_ref,
		telegram_chat_id = EXCLUDED.telegram_chat_id,
		whatsapp_enabled = EXCLUDED.whatsapp_enabled,
		whatsapp_secret_ref = EXCLUDED.whatsapp_secret_ref,
		whatsapp_phone_number_id = EXCLUDED.whatsapp_phone_number_id,
		whatsapp_default_recipient = EXCLUDED.whatsapp_default_recipient,
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
		config.TelegramEnabled,
		nullableString(config.TelegramSecretRef),
		nullableString(config.TelegramChatID),
		config.WhatsAppEnabled,
		nullableString(config.WhatsAppSecretRef),
		nullableString(config.WhatsAppPhoneNumberID),
		nullableString(config.WhatsAppDefaultRecipient),
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

func (s *Store) CreateMailingList(ctx context.Context, list *models.MailingList, members []string) error {
	if list == nil {
		return errors.New("mailing list required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

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

func (s *Store) UpdateMailingList(ctx context.Context, list *models.MailingList, members []string) error {
	if list == nil {
		return errors.New("mailing list required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

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

func (s *Store) UpsertNotificationSuppression(ctx context.Context, suppression *models.NotificationSuppression) error {
	if suppression == nil {
		return errors.New("notification suppression required")
	}

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

func (s *Store) ListNotificationSuppressions(ctx context.Context, tenantID string, limit, offset int, eventType, channel, severity string) ([]models.NotificationSuppression, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	clauses := []string{whereTenantID}
	if eventType != "" {
		args = append(args, eventType)
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", len(args)))
	}
	if channel != "" {
		args = append(args, channel)
		clauses = append(clauses, fmt.Sprintf("channel = $%d", len(args)))
	}
	if severity != "" {
		args = append(args, severity)
		clauses = append(clauses, fmt.Sprintf("severity = $%d", len(args)))
	}
	args = append(args, limit, offset)

	//nolint:gosec // #nosec G201 -- dynamic fragments are generated from fixed query clauses with positional placeholders.
	query := fmt.Sprintf(`SELECT dedup_key, tenant_id, channel, event_type, resource_id, severity,
		first_seen_at, last_seen_at, last_sent_at, suppressed_until, suppressed_count, last_payload_hash, updated_at
		FROM rbitr.notification_suppressions
		WHERE %s
		ORDER BY last_seen_at DESC
		LIMIT $%d OFFSET $%d`, strings.Join(clauses, " AND "), len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suppressions []models.NotificationSuppression
	for rows.Next() {
		var suppression models.NotificationSuppression
		var resourceID sql.NullString
		var lastSentAt sql.NullTime
		var suppressedUntil sql.NullTime
		var lastPayloadHash sql.NullString
		if err := rows.Scan(
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
			return nil, err
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
		suppressions = append(suppressions, suppression)
	}
	return suppressions, rows.Err()
}

// ListApprovalsExpiring returns approvals nearing expiry across all tenants.
// Used by the background scheduler for cross-tenant notification processing.
// Each result includes tenant_id for tenant-scoped notification dispatch.
func (s *Store) ListApprovalsExpiring(ctx context.Context, now time.Time, window time.Duration) ([]models.ApprovalRequest, error) {
	cutoff := now.Add(window)
	query := `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
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

// ListApprovalsExpired returns expired approvals across all tenants.
// Used by the background scheduler for cross-tenant expiry processing.
func (s *Store) ListApprovalsExpired(ctx context.Context, now time.Time) ([]models.ApprovalRequest, error) {
	query := `SELECT approval_request_id, tenant_id, agent_id, tool_id, action_type, request_hash, status,
		approval_token_hash, expires_at, created_at, policy_version, decided_at, decided_by, decision_comment,
		executed_at, executed_request_id, executed_decision_id, request_decision_id, action_summary,
		risk, rule_id, request_context, reasons
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

// Tenant management (Epic 7)

func (s *Store) CreateTenant(ctx context.Context, tenantID, name string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rbitr.tenants (tenant_id, name) VALUES ($1, $2)`,
		tenantID, name)
	return err
}

func (s *Store) SetTenantEnabled(ctx context.Context, tenantID string, enabled bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE rbitr.tenants SET enabled = $1 WHERE tenant_id = $2 AND deleted_at IS NULL`,
		enabled, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SoftDeleteTenant(ctx context.Context, tenantID string, deletedAt time.Time) error {
	if err := s.ensureAdminWritesAllowed(ctx); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE rbitr.tenants
		 SET deleted_at = $1, enabled = false
		 WHERE tenant_id = $2
		   AND deleted_at IS NULL`,
		deletedAt, tenantID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Tenant key lifecycle (Epic 7)

func (s *Store) CreateTenantKey(ctx context.Context, key *models.TenantKey) error {
	if key == nil {
		return errors.New("tenant key required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rbitr.tenant_keys (key_id, tenant_id, key_hash, key_prefix, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		key.KeyID, key.TenantID, key.KeyHash, key.KeyPrefix, key.CreatedAt)
	return err
}

func (s *Store) ListTenantKeys(ctx context.Context, tenantID string) ([]models.TenantKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key_id, tenant_id, key_prefix, created_at, revoked_at, rotated_at
		 FROM rbitr.tenant_keys
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.TenantKey
	for rows.Next() {
		var k models.TenantKey
		if err := rows.Scan(&k.KeyID, &k.TenantID, &k.KeyPrefix, &k.CreatedAt, &k.RevokedAt, &k.RotatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeTenantKey(ctx context.Context, tenantID, keyID string, revokedAt time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE rbitr.tenant_keys SET revoked_at = $1
		 WHERE key_id = $2 AND tenant_id = $3 AND revoked_at IS NULL`,
		revokedAt, keyID, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpgradeTenantKeyHash(ctx context.Context, oldKeyHash, newKeyHash string) error {
	if strings.TrimSpace(oldKeyHash) == "" || strings.TrimSpace(newKeyHash) == "" || oldKeyHash == newKeyHash {
		return nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE rbitr.tenant_keys
		 SET key_hash = $1
		 WHERE key_hash = $2 AND revoked_at IS NULL`,
		newKeyHash, oldKeyHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpgradeAdminKeyHash(ctx context.Context, oldKeyHash, newKeyHash string) error {
	if strings.TrimSpace(oldKeyHash) == "" || strings.TrimSpace(newKeyHash) == "" || oldKeyHash == newKeyHash {
		return nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE rbitr.admin_keys
		 SET key_hash = $1
		 WHERE key_hash = $2`,
		newKeyHash, oldKeyHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func hashKey(key string) string {
	return utils.HashTenantKey(key)
}

func parseOptionalPositiveInt64(value sql.NullString) (int64, bool) {
	if !value.Valid {
		return 0, false
	}
	trimmed := strings.TrimSpace(value.String)
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func parseOptionalScope(value sql.NullString) (string, bool) {
	if !value.Valid {
		return "", false
	}
	scope := strings.TrimSpace(value.String)
	if isRateLimitScope(scope) {
		return scope, true
	}
	return "", false
}

func isRateLimitScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "tenant", rateLimitScopeTenantAgent, rateLimitScopeTenantTool, defaultRateLimitScopeVal:
		return true
	default:
		return false
	}
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
		requestContextJSON []byte
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
		&requestContextJSON,
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
	if len(requestContextJSON) > 0 {
		if err := json.Unmarshal(requestContextJSON, &approval.RequestContext); err != nil {
			return models.ApprovalRequest{}, err
		}
	}
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &approval.Reasons); err != nil {
			return models.ApprovalRequest{}, err
		}
	}

	return approval, nil
}
