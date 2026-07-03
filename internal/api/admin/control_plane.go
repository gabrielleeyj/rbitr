package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/audit"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/license"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/policy/compiler"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type PolicyCreateRequest struct {
	PolicyVersion string `json:"policy_version"`
	RegoModule    string `json:"rego_module"`
	Notes         string `json:"notes"`
}

type PolicyRollbackRequest struct {
	PolicyVersion string `json:"policy_version"`
}

type PolicySimulationRequest struct {
	PolicyVersion string                     `json:"policy_version"`
	RegoModule    string                     `json:"rego_module"`
	Structured    *compiler.StructuredPolicy `json:"structured,omitempty"`
	Input         map[string]any             `json:"input"`
}

type ApprovalDecisionRequest struct {
	Comment string `json:"comment"`
}

type DefaultApprovalTTLRequest struct {
	Seconds int `json:"seconds"`
}

type AuditRetentionRequest struct {
	Days int `json:"days"`
}

type DefaultRateLimitRequest struct {
	PerMinute int64  `json:"per_minute"`
	PerDay    int64  `json:"per_day"`
	Scope     string `json:"scope"`
}

type EnforcementModeRequest struct {
	TenantID        string `json:"tenant_id"`
	EnforcementMode string `json:"enforcement_mode"`
}

type MCPPassthroughUpstreamRequest struct {
	TenantID string `json:"tenant_id"`
	ToolID   string `json:"tool_id"`
}

type BooleanSettingRequest struct {
	Enabled bool `json:"enabled"`
}

type NotificationConfigRequest struct {
	SlackWebhookEnabled        bool   `json:"slack_webhook_enabled"`
	SlackWebhookDefaultChannel string `json:"slack_webhook_default_channel"`
	SlackBotEnabled            bool   `json:"slack_bot_enabled"`
	SlackBotDefaultChannel     string `json:"slack_bot_default_channel"`
	EmailEnabled               bool   `json:"email_enabled"`
	EmailProvider              string `json:"email_provider"`
	EmailFrom                  string `json:"email_from"`
	EmailRegion                string `json:"email_region"`
	EmailDomain                string `json:"email_domain"`
	EmailDefaultMailingListID  string `json:"email_default_mailing_list_id"`
	TelegramEnabled            bool   `json:"telegram_enabled"`
	TelegramChatID             string `json:"telegram_chat_id"`
	WhatsAppEnabled            bool   `json:"whatsapp_enabled"`
	WhatsAppPhoneNumberID      string `json:"whatsapp_phone_number_id"`
	WhatsAppDefaultRecipient   string `json:"whatsapp_default_recipient"`
	NotifyApprovalExpiring     bool   `json:"notify_approval_expiring"`
	NotifyTokenAbuse           bool   `json:"notify_token_abuse"`
	NotifyPolicyInvalid        bool   `json:"notify_policy_invalid"`
}

type NotificationMetadataResponse struct {
	EventTypes []string `json:"event_types"`
	Severities []string `json:"severities"`
	Channels   []string `json:"channels"`
}

type approvalStatus string

const (
	approvalStatusPending   approvalStatus = "PENDING"
	approvalStatusApproved  approvalStatus = "APPROVED"
	approvalStatusDenied    approvalStatus = "DENIED"
	approvalStatusExecuting approvalStatus = "EXECUTING"
	approvalStatusExecuted  approvalStatus = "EXECUTED"
	approvalStatusFailed    approvalStatus = "FAILED"
	approvalStatusExpired   approvalStatus = "EXPIRED"
	approvalStatusRevoked   approvalStatus = "REVOKED"
	enforcementModeEnforce                 = "enforce"
	enforcementModeShadow                  = "shadow"
	defaultMinutes                         = 60
	defaultDays                            = 10000

	errApprovalNotFound       = "approval not found"
	errAdminWritesLocked      = "admin writes locked"
	auditFieldActivePolicyVer = "active_policy_version"
	auditFieldActionRisk      = "action_risk"
)

type NotificationConfigResponse struct {
	TenantID                   string    `json:"tenant_id"`
	SlackWebhookEnabled        bool      `json:"slack_webhook_enabled"`
	SlackWebhookConfigured     bool      `json:"slack_webhook_configured"`
	SlackWebhookDefaultChannel string    `json:"slack_webhook_default_channel"`
	SlackBotEnabled            bool      `json:"slack_bot_enabled"`
	SlackBotConfigured         bool      `json:"slack_bot_configured"`
	SlackBotDefaultChannel     string    `json:"slack_bot_default_channel"`
	EmailEnabled               bool      `json:"email_enabled"`
	EmailConfigured            bool      `json:"email_configured"`
	EmailProvider              string    `json:"email_provider"`
	EmailFrom                  string    `json:"email_from"`
	EmailRegion                string    `json:"email_region"`
	EmailDomain                string    `json:"email_domain"`
	EmailDefaultMailingListID  string    `json:"email_default_mailing_list_id"`
	TelegramEnabled            bool      `json:"telegram_enabled"`
	TelegramConfigured         bool      `json:"telegram_configured"`
	TelegramChatID             string    `json:"telegram_chat_id"`
	WhatsAppEnabled            bool      `json:"whatsapp_enabled"`
	WhatsAppConfigured         bool      `json:"whatsapp_configured"`
	WhatsAppPhoneNumberID      string    `json:"whatsapp_phone_number_id"`
	WhatsAppDefaultRecipient   string    `json:"whatsapp_default_recipient"`
	NotifyApprovalExpiring     bool      `json:"notify_approval_expiring"`
	NotifyTokenAbuse           bool      `json:"notify_token_abuse"`
	NotifyPolicyInvalid        bool      `json:"notify_policy_invalid"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type SecretRefRequest struct {
	SecretRef string `json:"secret_ref"`
}

type MailingListRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
}

type PolicyVersionsResponse struct {
	TenantID            string                 `json:"tenant_id"`
	ActivePolicyVersion string                 `json:"active_policy_version"`
	Versions            []models.PolicyVersion `json:"versions"`
}

type SettingsResponse struct {
	AdminWriteLock               bool   `json:"admin_write_lock"`
	DefaultApprovalTTLSeconds    int    `json:"default_approval_ttl_seconds"`
	AuditRetentionDays           int    `json:"audit_retention_days"`
	AuditRetentionDaysMax        int    `json:"audit_retention_days_max"` // Maximum allowed by license tier
	DisableXTenantKey            bool   `json:"disable_x_tenant_key"`
	FeatureRateLimiting          bool   `json:"feature_rate_limiting"`
	FeatureArgConstraints        bool   `json:"feature_arg_constraints"`
	DefaultRateLimitPerMinute    int64  `json:"default_rate_limit_per_minute"`
	DefaultRateLimitPerDay       int64  `json:"default_rate_limit_per_day"`
	DefaultRateLimitScope        string `json:"default_rate_limit_scope"`
	FeatureSessionTokens         bool   `json:"feature_session_tokens"`
	FeatureFileGovernance        bool   `json:"feature_file_governance"`
	SessionTokenTTLSeconds       int    `json:"session_token_ttl_seconds"`
	SecretProviderAWS            bool   `json:"secret_provider_aws"`
	SecretProviderGCP            bool   `json:"secret_provider_gcp"`
	SecretProviderVault          bool   `json:"secret_provider_vault"`
	SecretProviderAzure          bool   `json:"secret_provider_azure"`
	SSOEnabled                   bool   `json:"sso_enabled"`
	SSOIssuer                    string `json:"sso_issuer"`
	SSOClientID                  string `json:"sso_client_id"`
	SSOClientSecretRef           string `json:"sso_client_secret_ref"`
	SSORedirectURI               string `json:"sso_redirect_uri"`
	SSOAllowedDomains            string `json:"sso_allowed_domains"`
	SSODefaultScopes             string `json:"sso_default_scopes"`
	TenantID                     string `json:"tenant_id,omitempty"`
	EnforcementMode              string `json:"enforcement_mode,omitempty"`
	MCPPassthroughUpstreamToolID string `json:"mcp_passthrough_upstream_tool_id,omitempty"`
}

type HTTPConfig struct {
	BaseURL            string          `json:"base_url"`
	AuthType           string          `json:"auth_type"`
	AuthSet            bool            `json:"auth_set"`
	CredentialProvider string          `json:"credential_provider,omitempty"`
	CredentialConfig   json.RawMessage `json:"credential_config,omitempty"`
}

type MCPConfig struct {
	UpstreamURL     string          `json:"upstream_url"`
	Description     string          `json:"description"`
	InputSchemaJSON json.RawMessage `json:"input_schema_json"`
}

type ToolResponse struct {
	ToolID     string      `json:"tool_id"`
	TenantID   string      `json:"tenant_id"`
	HTTP       *HTTPConfig `json:"http,omitempty"`
	MCP        *MCPConfig  `json:"mcp,omitempty"`
	ArchivedAt *time.Time  `json:"archived_at,omitempty"`
	Source     string      `json:"source,omitempty"`
}

func (d *Dependencies) handleTenantList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeTenantsRead); err != nil {
		return err
	}
	items, err := d.Store.ListTenants(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tenants"})
	}
	return c.JSON(http.StatusOK, items)
}

func (d *Dependencies) handleTenantDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeTenantsRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	item, err := d.Store.GetTenant(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errTenantNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d *Dependencies) handleEvidenceList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeAuditRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
	}
	decision := c.QueryParam(fieldDecision)
	actionType := c.QueryParam("action_type")
	risk := c.QueryParam("risk")
	var since *time.Time
	if sinceParam := c.QueryParam("since"); sinceParam != "" {
		parsed, parseErr := time.Parse(time.RFC3339, sinceParam)
		if parseErr != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid since"})
		}
		since = &parsed
	}

	records, err := d.Store.ListEvidenceFiltered(c.Request().Context(), tenantID, decision, actionType, risk, since, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list evidence"})
	}
	exported := make([]models.ActionDecisionExport, 0, len(records))
	for i := range records {
		export := models.ActionDecisionExport{
			DecisionID:        records[i].DecisionID,
			RequestID:         records[i].RequestID,
			TenantID:          records[i].TenantID,
			AgentID:           records[i].AgentID,
			ToolID:            records[i].ToolID,
			ActionType:        records[i].ActionType,
			ActionRisk:        records[i].ActionRisk,
			ActionSummary:     records[i].ActionSummary,
			Decision:          records[i].Decision,
			DecisionVersion:   records[i].DecisionVersion,
			DecisionRisk:      records[i].DecisionRisk,
			RuleID:            records[i].RuleID,
			RulePriority:      records[i].RulePriority,
			Reasons:           records[i].Reasons,
			Constraints:       records[i].Constraints,
			Tags:              records[i].Tags,
			PolicyVersion:     records[i].PolicyVersion,
			Reason:            records[i].Reason,
			RequestHash:       records[i].RequestHash,
			ResponseHash:      records[i].ResponseHash,
			ApprovalRequestID: records[i].ApprovalRequestID,
			Timestamp:         records[i].CreatedAt,
		}
		if records[i].ApprovalRequestID != "" {
			if approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, records[i].ApprovalRequestID); err == nil {
				export.ApprovalStatus = approval.Status
				export.ApprovalDecidedAt = approval.DecidedAt
				export.ApprovalDecidedBy = approval.DecidedBy
				export.ApprovalComment = approval.DecisionComment
				export.ApprovalExecutedAt = approval.ExecutedAt
				export.ApprovalExecutedRequestID = approval.ExecutedRequestID
				export.ApprovalExecutedDecisionID = approval.ExecutedDecisionID
				export.ApprovalRequestDecisionID = approval.RequestDecisionID
			}
		}
		exported = append(exported, export)
	}
	return c.JSON(http.StatusOK, map[string]any{fieldTenantID: tenantID, "records": exported})
}

func (d *Dependencies) handleApprovalsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeApprovalsRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidOffset})
	}
	status := strings.ToUpper(strings.TrimSpace(c.QueryParam(fieldStatus)))
	if status != "" && !isApprovalStatus(status) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid status"})
	}

	approvals, err := d.Store.ListApprovalRequests(c.Request().Context(), tenantID, status, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list approvals"})
	}
	now := time.Now().UTC()
	for i := range approvals {
		if approvalExpired(&approvals[i], now) {
			if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvals[i].ApprovalRequestID, now); err == nil {
				approvals[i].Status = string(approvalStatusExpired)
				approvals[i].DecidedAt = &now
			}
		}
	}
	return c.JSON(http.StatusOK, approvals)
}

func (d *Dependencies) handleApprovalsPendingCount(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeApprovalsRead); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	count, err := d.Store.CountPendingApprovals(c.Request().Context(), tenantID, time.Now().UTC())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load pending approvals"})
	}
	return c.JSON(http.StatusOK, map[string]any{
		fieldTenantID:   tenantID,
		"pending_count": count,
	})
}

func (d *Dependencies) handleApprovalDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeApprovalsRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	approvalID := c.Param(fieldApprovalRequestID)
	approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errApprovalNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadApproval})
	}
	now := time.Now().UTC()
	if approvalExpired(&approval, now) {
		if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); err == nil {
			approval.Status = string(approvalStatusExpired)
			approval.DecidedAt = &now
		}
	}
	return c.JSON(http.StatusOK, approval)
}

func (d *Dependencies) handleApprovalApprove(c *echo.Context) error {
	return d.handleApprovalDecision(c, string(approvalStatusApproved), "APPROVAL.REQUEST.APPROVE")
}

func (d *Dependencies) handleApprovalDeny(c *echo.Context) error {
	return d.handleApprovalDecision(c, string(approvalStatusDenied), "APPROVAL.REQUEST.DENY")
}

func (d *Dependencies) handleApprovalRevoke(c *echo.Context) error {
	return d.handleApprovalDecision(c, string(approvalStatusRevoked), "APPROVAL.REQUEST.REVOKE")
}

func (d *Dependencies) handlePolicyVersions(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	versions, err := d.Store.ListPolicyVersions(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list policies"})
	}
	config, err := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadTenantConfig})
	}

	response := PolicyVersionsResponse{
		TenantID:            tenantID,
		ActivePolicyVersion: config.ActivePolicyVersion,
		Versions:            versions,
	}
	return c.JSON(http.StatusOK, response)
}

func (d *Dependencies) handlePolicyVersionGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	version := c.Param(fieldPolicyVersion)
	item, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errPolicyVersionNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load policy"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d *Dependencies) handlePolicyCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesWrite)
	if err != nil {
		return err
	}

	var payload PolicyCreateRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.PolicyVersion == "" || payload.RegoModule == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "policy_version and rego_module required"})
	}
	if err := validateRegoModule(payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if _, err := opa.PrepareQuery(c.Request().Context(), payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":     "rego compilation failed",
			fieldDetail: err.Error(),
		})
	}

	tenantID := c.Param(fieldTenantID)
	if err := d.Store.CreatePolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion, payload.RegoModule, adminKey.AdminKeyID, payload.Notes); err != nil {
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": errAdminWritesLocked})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to create policy",
			fieldDetail: err.Error(),
		})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.CREATE", "POLICY.VERSION", payload.PolicyVersion, nil, map[string]any{
		fieldPolicyVersion: payload.PolicyVersion,
		"created_by":       adminKey.AdminKeyID,
		"notes":            payload.Notes,
		"rego_sha256":      utils.HashString(payload.RegoModule),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit policy create",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusCreated)
}

func (d *Dependencies) handlePolicyPublish(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesPub)
	if err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	version := c.Param(fieldPolicyVersion)
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.Store.PublishPolicyVersion(c.Request().Context(), tenantID, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errPolicyVersionNotFound})
		}
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": errAdminWritesLocked})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to publish policy",
			fieldDetail: err.Error(),
		})
	}
	d.invalidateTenantCaches(tenantID)
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", version, map[string]any{
		auditFieldActivePolicyVer: before.ActivePolicyVersion,
	}, map[string]any{
		auditFieldActivePolicyVer: after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit policy publish",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handlePolicyRollback(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesRB)
	if err != nil {
		return err
	}

	var payload PolicyRollbackRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.Store.RollbackPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errPolicyVersionNotFound})
		}
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": errAdminWritesLocked})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to rollback policy",
			fieldDetail: err.Error(),
		})
	}
	d.invalidateTenantCaches(tenantID)
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	target := payload.PolicyVersion
	if target == "" {
		target = after.ActivePolicyVersion
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.ROLLBACK", "POLICY.ACTIVE", target, map[string]any{
		auditFieldActivePolicyVer: before.ActivePolicyVersion,
	}, map[string]any{
		auditFieldActivePolicyVer: after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit policy rollback",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handlePolicySimulate(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesSim); err != nil {
		return err
	}

	var payload PolicySimulationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.Input == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "input required"})
	}

	tenantID := c.Param(fieldTenantID)
	regoModule := payload.RegoModule
	if payload.Structured != nil {
		compiled, err := compiler.Compile(payload.Structured)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error":     "structured policy invalid",
				fieldDetail: err.Error(),
			})
		}
		regoModule = compiled
	}
	//nolint:nestif // Selecting policy source inline keeps version/active fallback behavior explicit.
	if regoModule == "" {
		if payload.PolicyVersion != "" {
			version, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion)
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": errPolicyVersionNotFound})
			}
			regoModule = version.RegoModule
		} else {
			policy, err := d.Store.GetPolicy(c.Request().Context(), tenantID)
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "active policy not found"})
			}
			regoModule = policy.RegoModule
		}
	}
	if err := validateRegoModule(regoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	engine := opa.NewEngine(regoModule)
	result, err := engine.Evaluate(c.Request().Context(), payload.Input)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":     "policy evaluation failed",
			fieldDetail: err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		fieldDecision: map[string]any{
			"version":       result.Version,
			fieldDecision:   result.Decision,
			"risk":          result.Risk,
			"rule":          map[string]any{"id": result.Rule.ID, "priority": result.Rule.Priority},
			"reasons":       simulationReasons(result.Reasons),
			"constraints":   result.Constraints,
			"tags":          result.Tags,
			"matched_rules": simulationMatchedRules(result.MatchedRules),
		},
	})
}

func simulationReasons(reasons []opa.Reason) []map[string]any {
	out := make([]map[string]any, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, map[string]any{
			"code":       reason.Code,
			fieldMessage: reason.Message,
		})
	}
	return out
}

func simulationMatchedRules(rules []opa.MatchedRule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		item := map[string]any{
			"rule_id":  rule.RuleID,
			"priority": rule.Priority,
			"effect":   rule.Effect,
		}
		if len(rule.Reasons) > 0 {
			item["reasons"] = simulationReasons(rule.Reasons)
		}
		if len(rule.ConstraintsSummary) > 0 {
			item["constraints_summary"] = rule.ConstraintsSummary
		}
		out = append(out, item)
	}
	return out
}

func (d *Dependencies) handleRiskOverridesList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	overrides, err := d.Store.ListRiskOverrides(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list overrides"})
	}
	return c.JSON(http.StatusOK, overrides)
}

func (d *Dependencies) handleRiskOverrideDelete(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesWrite)
	if err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	actionType := c.Param("action_type")
	beforeRisk, _ := d.Store.GetRiskOverride(c.Request().Context(), tenantID, actionType)
	if err := d.Store.DeleteRiskOverride(c.Request().Context(), tenantID, actionType); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete override"})
	}
	d.invalidateTenantCaches(tenantID)
	if err := d.emitAuditEvent(c, adminKey, tenantID, "RISK_OVERRIDE.DELETE", "RISK_OVERRIDE", actionType, map[string]any{
		"action_type":        actionType,
		auditFieldActionRisk: beforeRisk,
	}, nil); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit override delete",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleToolsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeToolsRead); err != nil {
		return err
	}

	tenantID := c.Param(fieldTenantID)
	includeArchived := c.QueryParam("include_archived") == "true"
	tools, err := d.Store.ListTools(c.Request().Context(), tenantID, includeArchived, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tools"})
	}
	response := make([]ToolResponse, 0, len(tools))
	for i := range tools {
		toolResponse := ToolResponse{
			ToolID:     tools[i].ToolID,
			TenantID:   tools[i].TenantID,
			ArchivedAt: tools[i].ArchivedAt,
			Source:     tools[i].Source,
		}

		// Add HTTP config if base_url is set
		if tools[i].BaseURL != "" {
			toolResponse.HTTP = &HTTPConfig{
				BaseURL:  tools[i].BaseURL,
				AuthType: tools[i].AuthType,
				AuthSet:  tools[i].AuthValue != "",
			}
		}

		// Add MCP config if any MCP fields are set
		if tools[i].MCPUpstreamURL != "" || tools[i].Description != "" || len(tools[i].InputSchemaJSON) > 0 {
			toolResponse.MCP = &MCPConfig{
				UpstreamURL:     tools[i].MCPUpstreamURL,
				Description:     tools[i].Description,
				InputSchemaJSON: tools[i].InputSchemaJSON,
			}
		}

		response = append(response, toolResponse)
	}
	return c.JSON(http.StatusOK, response)
}

func (d *Dependencies) handleSettingsGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeSettingsRead); err != nil {
		return err
	}
	tenantID := strings.TrimSpace(c.QueryParam(fieldTenantID))
	locked, err := d.Store.GetAdminWriteLock(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
	}
	defaultTTL := 900
	if value, err := d.Store.GetDefaultApprovalTTLSeconds(c.Request().Context()); err == nil && value > 0 {
		defaultTTL = value
	}
	retentionDays := 365
	if value, err := d.Store.GetAuditRetentionDays(c.Request().Context()); err == nil && value > 0 {
		retentionDays = value
	}
	disableXTenantKey := d.Config.DisableXTenantKey
	if value, err := d.Store.GetDisableXTenantKey(c.Request().Context()); err == nil {
		disableXTenantKey = value
	}
	featureRateLimiting := d.Config.FeatureRateLimiting
	if value, err := d.Store.GetFeatureRateLimiting(c.Request().Context()); err == nil {
		featureRateLimiting = value
	}
	featureArgConstraints := d.Config.FeatureArgConstraints
	if value, err := d.Store.GetFeatureArgConstraints(c.Request().Context()); err == nil {
		featureArgConstraints = value
	}
	featureSessionTokens := d.Config.FeatureSessionTokens
	if value, err := d.Store.GetFeatureSessionTokens(c.Request().Context()); err == nil {
		featureSessionTokens = value
	}
	featureFileGovernance := d.Config.FeatureFileGovernance
	if value, err := d.Store.GetFeatureFileGovernance(c.Request().Context()); err == nil {
		featureFileGovernance = value
	}
	sessionTokenTTLSeconds := int(d.Config.SessionTokenTTL.Seconds())
	if value, err := d.Store.GetSessionTokenTTLSeconds(c.Request().Context()); err == nil && value > 0 {
		sessionTokenTTLSeconds = value
	}
	secretProviderAWS := d.Config.SecretProviderAWS
	if value, err := d.Store.GetSecretProviderAWS(c.Request().Context()); err == nil {
		secretProviderAWS = value
	}
	secretProviderGCP := d.Config.SecretProviderGCP
	if value, err := d.Store.GetSecretProviderGCP(c.Request().Context()); err == nil {
		secretProviderGCP = value
	}
	secretProviderVault := d.Config.SecretProviderVault
	if value, err := d.Store.GetSecretProviderVault(c.Request().Context()); err == nil {
		secretProviderVault = value
	}
	secretProviderAzure := d.Config.SecretProviderAzure
	if value, err := d.Store.GetSecretProviderAzure(c.Request().Context()); err == nil {
		secretProviderAzure = value
	}
	defaultRateLimit := models.RateLimitConfig{
		PerMinute: defaultMinutes,
		PerDay:    defaultDays,
		Scope:     rateLimitScopeTenantAgentTool,
	}
	if value, err := d.Store.GetDefaultRateLimitConfig(c.Request().Context()); err == nil {
		defaultRateLimit = value
	}
	ssoConfig, _ := d.Store.GetSSOConfig(c.Request().Context())

	// Get license tier's maximum allowed audit retention
	retentionDaysMax := license.DefaultAuditRetentionDays
	if d.LicenseProvider != nil {
		ent := d.LicenseProvider.Entitlements()
		retentionDaysMax = ent.AuditRetentionDays
	}

	enforcementMode := enforcementModeEnforce
	mcpPassthroughUpstreamToolID := ""
	if tenantID != "" {
		tenantConfig, err := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": errTenantConfigNotFound})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadTenantConfig})
		}
		if mode := strings.TrimSpace(tenantConfig.EnforcementMode); mode == string(enforcementModeShadow) {
			enforcementMode = mode
		}
		mcpPassthroughUpstreamToolID = strings.TrimSpace(tenantConfig.MCPPassthroughUpstreamToolID)
	}
	return c.JSON(http.StatusOK, SettingsResponse{
		AdminWriteLock:               locked,
		DefaultApprovalTTLSeconds:    defaultTTL,
		AuditRetentionDays:           retentionDays,
		AuditRetentionDaysMax:        retentionDaysMax,
		DisableXTenantKey:            disableXTenantKey,
		FeatureRateLimiting:          featureRateLimiting,
		FeatureArgConstraints:        featureArgConstraints,
		DefaultRateLimitPerMinute:    defaultRateLimit.PerMinute,
		DefaultRateLimitPerDay:       defaultRateLimit.PerDay,
		DefaultRateLimitScope:        defaultRateLimit.Scope,
		FeatureSessionTokens:         featureSessionTokens,
		FeatureFileGovernance:        featureFileGovernance,
		SessionTokenTTLSeconds:       sessionTokenTTLSeconds,
		SecretProviderAWS:            secretProviderAWS,
		SecretProviderGCP:            secretProviderGCP,
		SecretProviderVault:          secretProviderVault,
		SecretProviderAzure:          secretProviderAzure,
		SSOEnabled:                   ssoConfig.Enabled,
		SSOIssuer:                    ssoConfig.Issuer,
		SSOClientID:                  ssoConfig.ClientID,
		SSOClientSecretRef:           ssoConfig.ClientSecretRef,
		SSORedirectURI:               ssoConfig.RedirectURI,
		SSOAllowedDomains:            ssoConfig.AllowedDomains,
		SSODefaultScopes:             ssoConfig.DefaultScopes,
		TenantID:                     tenantID,
		EnforcementMode:              enforcementMode,
		MCPPassthroughUpstreamToolID: mcpPassthroughUpstreamToolID,
	})
}

func (d *Dependencies) handleAuditList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeAuditRead); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidOffset})
	}
	tenantID := c.Param(fieldTenantID)
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidFrom})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidTo})
	}
	from = applyVisibilityFloor(from, d.auditVisibilityFloor())
	events, err := d.Store.ListAuditEvents(c.Request().Context(), tenantID, limit, offset, action, resourceType, actorID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to list audit events",
			fieldDetail: err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d *Dependencies) handleAuditResourceTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeAuditRead); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	values, err := d.Store.ListAuditResourceTypes(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list audit resource types"})
	}
	return c.JSON(http.StatusOK, map[string][]string{
		"resource_types": values,
	})
}

func (d *Dependencies) handleAuditExport(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeAuditExport); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	return d.handleAuditExportResponse(c, tenantID)
}

func (d *Dependencies) handleAuditExportResponse(c *echo.Context, tenantID string) error {
	format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))
	if format == "" {
		format = "json"
	}
	includeDetails := strings.EqualFold(c.QueryParam("include_details"), "true")
	all := strings.EqualFold(c.QueryParam("all"), "true")
	var (
		limit  int
		offset int
		err    error
	)
	if !all {
		limit, err = parseExportLimit(c.QueryParam("limit"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
		}
		offset, err = parseOffset(c.QueryParam("offset"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidOffset})
		}
	}
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidFrom})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidTo})
	}
	from = applyVisibilityFloor(from, d.auditVisibilityFloor())
	events, err := d.Store.ListAuditEventsExport(c.Request().Context(), tenantID, limit, offset, action, resourceType, actorID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to export audit events"})
	}
	if !includeDetails {
		for i := range events {
			events[i].Before = nil
			events[i].After = nil
		}
	}

	switch format {
	case "csv":
		c.Response().Header().Set("Content-Type", "text/csv")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=audit_export.csv")
		writer := csv.NewWriter(c.Response())
		if err := writeAuditCSV(writer, events, includeDetails); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write csv"})
		}
		writer.Flush()
		return nil
	case "json":
		return c.JSON(http.StatusOK, events)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid format"})
	}
}

func (d *Dependencies) handleAuditListAll(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeAuditRead); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidOffset})
	}
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidFrom})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidTo})
	}
	from = applyVisibilityFloor(from, d.auditVisibilityFloor())
	events, err := d.Store.ListAuditEvents(c.Request().Context(), "", limit, offset, action, resourceType, actorID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to list audit events",
			fieldDetail: err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d *Dependencies) handleNotificationConfigGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifRead); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "notification config not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	return c.JSON(http.StatusOK, NotificationConfigResponse{
		TenantID:                   config.TenantID,
		SlackWebhookEnabled:        config.SlackWebhookEnabled,
		SlackWebhookConfigured:     config.SlackWebhookSecretRef != "",
		SlackWebhookDefaultChannel: config.SlackWebhookDefaultChannel,
		SlackBotEnabled:            config.SlackBotEnabled,
		SlackBotConfigured:         config.SlackBotSecretRef != "",
		SlackBotDefaultChannel:     config.SlackBotDefaultChannel,
		EmailEnabled:               config.EmailEnabled,
		EmailConfigured:            config.EmailSecretRef != "",
		EmailProvider:              config.EmailProvider,
		EmailFrom:                  config.EmailFrom,
		EmailRegion:                config.EmailRegion,
		EmailDomain:                config.EmailDomain,
		EmailDefaultMailingListID:  config.EmailDefaultMailingListID,
		TelegramEnabled:            config.TelegramEnabled,
		TelegramConfigured:         config.TelegramSecretRef != "",
		TelegramChatID:             config.TelegramChatID,
		WhatsAppEnabled:            config.WhatsAppEnabled,
		WhatsAppConfigured:         config.WhatsAppSecretRef != "",
		WhatsAppPhoneNumberID:      config.WhatsAppPhoneNumberID,
		WhatsAppDefaultRecipient:   config.WhatsAppDefaultRecipient,
		NotifyApprovalExpiring:     config.NotifyApprovalExpiring,
		NotifyTokenAbuse:           config.NotifyTokenAbuse,
		NotifyPolicyInvalid:        config.NotifyPolicyInvalid,
		UpdatedAt:                  config.UpdatedAt,
	})
}

func (d *Dependencies) handleNotificationConfigUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload NotificationConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.SlackBotEnabled && payload.SlackBotDefaultChannel == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "slack_bot_default_channel required"})
	}
	if payload.EmailEnabled && payload.EmailProvider == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_provider required"})
	}
	if payload.EmailEnabled && payload.EmailFrom == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_from required"})
	}
	if payload.EmailEnabled && strings.EqualFold(payload.EmailProvider, "ses") && payload.EmailRegion == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_region required for ses"})
	}
	if payload.EmailEnabled && strings.EqualFold(payload.EmailProvider, "mailgun") && payload.EmailDomain == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_domain required for mailgun"})
	}
	if payload.TelegramEnabled && payload.TelegramChatID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "telegram_chat_id required"})
	}
	if payload.WhatsAppEnabled && payload.WhatsAppPhoneNumberID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "whatsapp_phone_number_id required"})
	}
	if payload.WhatsAppEnabled && payload.WhatsAppDefaultRecipient == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "whatsapp_default_recipient required"})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if payload.EmailEnabled {
		provider := strings.ToLower(payload.EmailProvider)
		if (provider == "sendgrid" || provider == "mailgun") && before.EmailSecretRef == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "email_secret_ref required"})
		}
	}
	config := models.NotificationConfig{
		TenantID:                   tenantID,
		SlackWebhookEnabled:        payload.SlackWebhookEnabled,
		SlackWebhookSecretRef:      before.SlackWebhookSecretRef,
		SlackWebhookDefaultChannel: payload.SlackWebhookDefaultChannel,
		SlackBotEnabled:            payload.SlackBotEnabled,
		SlackBotSecretRef:          before.SlackBotSecretRef,
		SlackBotDefaultChannel:     payload.SlackBotDefaultChannel,
		SlackBotSigningSecretRef:   before.SlackBotSigningSecretRef,
		EmailEnabled:               payload.EmailEnabled,
		EmailProvider:              payload.EmailProvider,
		EmailSecretRef:             before.EmailSecretRef,
		EmailFrom:                  payload.EmailFrom,
		EmailRegion:                payload.EmailRegion,
		EmailDomain:                payload.EmailDomain,
		EmailDefaultMailingListID:  payload.EmailDefaultMailingListID,
		TelegramEnabled:            payload.TelegramEnabled,
		TelegramSecretRef:          before.TelegramSecretRef,
		TelegramChatID:             payload.TelegramChatID,
		WhatsAppEnabled:            payload.WhatsAppEnabled,
		WhatsAppSecretRef:          before.WhatsAppSecretRef,
		WhatsAppPhoneNumberID:      payload.WhatsAppPhoneNumberID,
		WhatsAppDefaultRecipient:   payload.WhatsAppDefaultRecipient,
		NotifyApprovalExpiring:     payload.NotifyApprovalExpiring,
		NotifyTokenAbuse:           payload.NotifyTokenAbuse,
		NotifyPolicyInvalid:        payload.NotifyPolicyInvalid,
	}
	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update notification config"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.UPDATE", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		"slack_webhook_enabled": before.SlackWebhookEnabled,
		"slack_bot_enabled":     before.SlackBotEnabled,
		"email_enabled":         before.EmailEnabled,
		"email_provider":        before.EmailProvider,
		"telegram_enabled":      before.TelegramEnabled,
		"whatsapp_enabled":      before.WhatsAppEnabled,
	}, map[string]any{
		"slack_webhook_enabled": payload.SlackWebhookEnabled,
		"slack_bot_enabled":     payload.SlackBotEnabled,
		"email_enabled":         payload.EmailEnabled,
		"email_provider":        payload.EmailProvider,
		"telegram_enabled":      payload.TelegramEnabled,
		"whatsapp_enabled":      payload.WhatsAppEnabled,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit notification update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationSlackSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidSecretRef})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.SlackWebhookSecretRef != "" || before.SlackBotSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.SlackWebhookSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update slack secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.SLACK_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		fieldConfigured: beforeConfigured,
	}, map[string]any{
		fieldConfigured: true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit slack secret ref",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationEmailSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidSecretRef})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.EmailSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.EmailSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update email secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.EMAIL_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		fieldConfigured: beforeConfigured,
	}, map[string]any{
		fieldConfigured: true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit email secret ref",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleMailingListsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifRead); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	lists, err := d.Store.ListMailingLists(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list mailing lists"})
	}
	return c.JSON(http.StatusOK, lists)
}

func (d *Dependencies) handleMailingListCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload MailingListRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
	}
	for _, email := range payload.Members {
		if !strings.Contains(email, "@") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid member email"})
		}
	}

	tenantID := c.Param(fieldTenantID)
	list := models.MailingList{
		MailingListID: uuid.NewString(),
		TenantID:      tenantID,
		Name:          payload.Name,
		Description:   payload.Description,
	}
	if err := d.Store.CreateMailingList(c.Request().Context(), &list, payload.Members); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.CREATE", "MAILING_LIST", list.MailingListID, map[string]any{}, map[string]any{
		fieldName:      payload.Name,
		"description":  payload.Description,
		"member_count": len(payload.Members),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit mailing list create",
			fieldDetail: err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, list)
}

func (d *Dependencies) handleMailingListUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload MailingListRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
	}
	for _, email := range payload.Members {
		if !strings.Contains(email, "@") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid member email"})
		}
	}

	tenantID := c.Param(fieldTenantID)
	listID := c.Param("mailing_list_id")
	before, _ := d.Store.GetMailingList(c.Request().Context(), tenantID, listID)
	list := models.MailingList{
		MailingListID: listID,
		TenantID:      tenantID,
		Name:          payload.Name,
		Description:   payload.Description,
	}
	if err := d.Store.UpdateMailingList(c.Request().Context(), &list, payload.Members); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.UPDATE", "MAILING_LIST", listID, map[string]any{
		fieldName:     before.Name,
		"description": before.Description,
	}, map[string]any{
		fieldName:      payload.Name,
		"description":  payload.Description,
		"member_count": len(payload.Members),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit mailing list update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleMailingListDelete(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	listID := c.Param("mailing_list_id")
	before, _ := d.Store.GetMailingList(c.Request().Context(), tenantID, listID)
	if err := d.Store.DeleteMailingList(c.Request().Context(), tenantID, listID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.DELETE", "MAILING_LIST", listID, map[string]any{
		fieldName:     before.Name,
		"description": before.Description,
	}, map[string]any{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit mailing list delete",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTestSlack(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifTest); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": errNotifNotConfigured})
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errNotifConfigMissing})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	if !config.SlackWebhookEnabled || config.SlackWebhookSecretRef == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "slack webhook not configured"})
	}
	msg := notifications.NotificationMessage{
		Title:  "Slack notification test",
		Body:   testNotificationBody,
		Fields: map[string]string{fieldTenant: tenantID},
	}
	if err := d.Notifications.Send(c.Request().Context(), tenantID, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventNotificationsTest,
		Severity:   severityInfo,
		ResourceID: resourceTest,
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send slack test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTestSlackBot(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifTest); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": errNotifNotConfigured})
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errNotifConfigMissing})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	if !config.SlackBotEnabled || config.SlackBotSecretRef == "" || config.SlackBotDefaultChannel == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "slack bot not configured"})
	}
	token, err := d.Notifications.ResolveSecret(c.Request().Context(), config.SlackBotSecretRef)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve slack bot secret"})
	}
	engine := notifications.NewEngine(d.Store, map[string]notifications.Notifier{
		notifications.SlackBotChannel: notifications.NewSlackBotNotifier(token, config.SlackBotDefaultChannel, nil, ""),
	}, d.Notifications.Cooldown, d.Metrics)
	msg := notifications.NotificationMessage{
		Title:  "Slack bot notification test",
		Body:   testNotificationBody,
		Fields: map[string]string{fieldTenant: tenantID},
	}
	if err := engine.Send(c.Request().Context(), notifications.SlackBotChannel, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventNotificationsTest,
		Severity:   severityInfo,
		ResourceID: resourceTest,
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send slack bot test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTestEmail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifTest); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": errNotifNotConfigured})
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errNotifConfigMissing})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	if !config.EmailEnabled || config.EmailProvider == "" || config.EmailFrom == "" || config.EmailDefaultMailingListID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email not configured"})
	}
	msg := notifications.NotificationMessage{
		Title:  "Email notification test",
		Body:   testNotificationBody,
		Fields: map[string]string{fieldTenant: tenantID},
	}
	if err := d.Notifications.Send(c.Request().Context(), tenantID, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventNotificationsTest,
		Severity:   severityInfo,
		ResourceID: resourceTest,
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send email test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTelegramSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidSecretRef})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.TelegramSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.TelegramSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update telegram secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.TELEGRAM_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		fieldConfigured: beforeConfigured,
	}, map[string]any{
		fieldConfigured: true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit telegram secret ref",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationWhatsAppSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeNotifWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidSecretRef})
	}

	tenantID := c.Param(fieldTenantID)
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.WhatsAppSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.WhatsAppSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), &config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update whatsapp secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.WHATSAPP_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		fieldConfigured: beforeConfigured,
	}, map[string]any{
		fieldConfigured: true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit whatsapp secret ref",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTestTelegram(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifTest); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": errNotifNotConfigured})
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errNotifConfigMissing})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	if !config.TelegramEnabled || config.TelegramSecretRef == "" || config.TelegramChatID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "telegram not configured"})
	}
	botToken, err := d.Notifications.ResolveSecret(c.Request().Context(), config.TelegramSecretRef)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve telegram secret"})
	}
	engine := notifications.NewEngine(d.Store, map[string]notifications.Notifier{
		notifications.TelegramChannel: notifications.NewTelegramNotifier(botToken, config.TelegramChatID),
	}, d.Notifications.Cooldown, d.Metrics)
	msg := notifications.NotificationMessage{
		Title:  "Telegram notification test",
		Body:   testNotificationBody,
		Fields: map[string]string{fieldTenant: tenantID},
	}
	if err := engine.Send(c.Request().Context(), notifications.TelegramChannel, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventNotificationsTest,
		Severity:   severityInfo,
		ResourceID: resourceTest,
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send telegram test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationTestWhatsApp(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifTest); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": errNotifNotConfigured})
	}
	tenantID := c.Param(fieldTenantID)
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errNotifConfigMissing})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadNotifConfig})
	}
	if !config.WhatsAppEnabled || config.WhatsAppSecretRef == "" || config.WhatsAppPhoneNumberID == "" || config.WhatsAppDefaultRecipient == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "whatsapp not configured"})
	}
	accessToken, err := d.Notifications.ResolveSecret(c.Request().Context(), config.WhatsAppSecretRef)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve whatsapp secret"})
	}
	engine := notifications.NewEngine(d.Store, map[string]notifications.Notifier{
		notifications.WhatsAppChannel: notifications.NewWhatsAppNotifier(accessToken, config.WhatsAppPhoneNumberID, config.WhatsAppDefaultRecipient),
	}, d.Notifications.Cooldown, d.Metrics)
	msg := notifications.NotificationMessage{
		Title:  "WhatsApp notification test",
		Body:   testNotificationBody,
		Fields: map[string]string{fieldTenant: tenantID},
	}
	if err := engine.Send(c.Request().Context(), notifications.WhatsAppChannel, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventNotificationsTest,
		Severity:   severityInfo,
		ResourceID: resourceTest,
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send whatsapp test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleNotificationSuppressions(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifRead); err != nil {
		return err
	}
	tenantID := c.Param(fieldTenantID)
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidLimit})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidOffset})
	}
	eventType := strings.ToUpper(strings.TrimSpace(c.QueryParam("event_type")))
	channel := strings.TrimSpace(c.QueryParam("channel"))
	severity := strings.ToUpper(strings.TrimSpace(c.QueryParam("severity")))
	items, err := d.Store.ListNotificationSuppressions(c.Request().Context(), tenantID, limit, offset, eventType, channel, severity)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list suppressions"})
	}
	return c.JSON(http.StatusOK, items)
}

func (d *Dependencies) handleNotificationEventTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeNotifRead); err != nil {
		return err
	}
	channels := []string{
		notifications.SlackWebhookChannel,
		notifications.SlackBotChannel,
		notifications.EmailChannel,
		notifications.TelegramChannel,
		notifications.WhatsAppChannel,
	}
	sort.Strings(channels)
	return c.JSON(http.StatusOK, NotificationMetadataResponse{
		EventTypes: notifications.EventTypes(),
		Severities: notifications.Severities(),
		Channels:   channels,
	})
}

func (d *Dependencies) handleActionTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopePoliciesRead); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string][]string{
		"action_types": classification.ActionTypes(),
	})
}

func (d *Dependencies) handleDefaultApprovalTTLUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload DefaultApprovalTTLRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	if payload.Seconds < 60 || payload.Seconds > 86400 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "seconds must be between 60 and 86400"})
	}

	beforeTTL, _ := d.Store.GetDefaultApprovalTTLSeconds(c.Request().Context())
	if err := d.Store.SetDefaultApprovalTTLSeconds(c.Request().Context(), payload.Seconds); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update default approval ttl"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.APPROVAL_TTL_DEFAULT.SET", "SETTINGS", "default_approval_ttl_seconds", map[string]any{
		fieldValue: beforeTTL,
	}, map[string]any{
		fieldValue: payload.Seconds,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit approval ttl update",
			fieldDetail: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleAuditRetentionUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload AuditRetentionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	if payload.Days < 30 || payload.Days > 3650 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "days must be between 30 and 3650"})
	}

	// Validate against license tier maximum
	if d.LicenseProvider != nil {
		ent := d.LicenseProvider.Entitlements()
		maxAllowed := ent.AuditRetentionDays
		if !license.IsUnlimited(maxAllowed) && payload.Days > maxAllowed {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error":       "audit retention exceeds license tier maximum",
				"max_allowed": strconv.Itoa(maxAllowed),
			})
		}
	}

	beforeValue, _ := d.Store.GetAuditRetentionDays(c.Request().Context())
	if err := d.Store.SetAuditRetentionDays(c.Request().Context(), payload.Days); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update audit retention"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.AUDIT_RETENTION.SET", "SETTINGS", "audit_retention_days", map[string]any{
		fieldValue: beforeValue,
	}, map[string]any{
		fieldValue: payload.Days,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit retention update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleDefaultRateLimitUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload DefaultRateLimitRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	payload.Scope = strings.TrimSpace(payload.Scope)
	if payload.PerMinute <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "per_minute must be > 0"})
	}
	if payload.PerDay <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "per_day must be > 0"})
	}
	if payload.PerDay < payload.PerMinute {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "per_day must be >= per_minute"})
	}
	if !isRateLimitScope(payload.Scope) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "scope must be one of tenant, tenant_agent, tenant_tool, tenant_agent_tool",
		})
	}

	beforeValue, _ := d.Store.GetDefaultRateLimitConfig(c.Request().Context())
	if err := d.Store.SetDefaultRateLimitConfig(c.Request().Context(), payload.PerMinute, payload.PerDay, payload.Scope); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update default rate limit config"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.RATE_LIMIT_DEFAULT.SET", "SETTINGS", "default_rate_limit_config", map[string]any{
		"per_minute": beforeValue.PerMinute,
		"per_day":    beforeValue.PerDay,
		fieldScope:   beforeValue.Scope,
	}, map[string]any{
		"per_minute": payload.PerMinute,
		"per_day":    payload.PerDay,
		fieldScope:   payload.Scope,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit default rate limit update",
			fieldDetail: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleEnforcementModeUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload EnforcementModeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	payload.TenantID = strings.TrimSpace(payload.TenantID)
	payload.EnforcementMode = strings.TrimSpace(strings.ToLower(payload.EnforcementMode))
	if payload.TenantID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "tenant_id required"})
	}
	if payload.EnforcementMode != enforcementModeEnforce && payload.EnforcementMode != enforcementModeShadow {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "enforcement_mode must be enforce or shadow"})
	}

	beforeMode := enforcementModeEnforce
	if existingConfig, err := d.Store.GetTenantConfig(c.Request().Context(), payload.TenantID); err == nil {
		if mode := strings.TrimSpace(existingConfig.EnforcementMode); mode == enforcementModeShadow {
			beforeMode = mode
		}
	}

	if err := d.Store.SetTenantEnforcementMode(c.Request().Context(), payload.TenantID, payload.EnforcementMode); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errTenantConfigNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update enforcement mode"})
	}
	if err := d.emitAuditEvent(c, adminKey, payload.TenantID, "SETTINGS.ENFORCEMENT_MODE.SET", "SETTINGS", "enforcement_mode", map[string]any{
		fieldTenantID: payload.TenantID,
		fieldValue:    beforeMode,
	}, map[string]any{
		fieldTenantID: payload.TenantID,
		fieldValue:    payload.EnforcementMode,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit enforcement mode update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleMCPPassthroughUpstreamUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload MCPPassthroughUpstreamRequest
	if decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload); decodeErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}
	payload.TenantID = strings.TrimSpace(payload.TenantID)
	payload.ToolID = strings.TrimSpace(payload.ToolID)
	if payload.TenantID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "tenant_id required"})
	}

	tenantConfig, err := d.Store.GetTenantConfig(c.Request().Context(), payload.TenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errTenantConfigNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadTenantConfig})
	}
	beforeToolID := strings.TrimSpace(tenantConfig.MCPPassthroughUpstreamToolID)

	if payload.ToolID != "" {
		tool, err := d.Store.GetTool(c.Request().Context(), payload.TenantID, payload.ToolID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": errToolNotFound})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tool"})
		}
		if tool.Transport != "mcp_streamable_http" || strings.TrimSpace(tool.MCPUpstreamURL) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "tool_id must reference an MCP tool with mcp_upstream_url"})
		}
	}

	if err := d.Store.SetTenantMCPPassthroughUpstreamToolID(c.Request().Context(), payload.TenantID, payload.ToolID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errTenantConfigNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update mcp pass-through upstream"})
	}
	if err := d.emitAuditEvent(c, adminKey, payload.TenantID, "SETTINGS.MCP_PASSTHROUGH_UPSTREAM.SET", "SETTINGS", "mcp_passthrough_upstream_tool_id", map[string]any{
		fieldTenantID: payload.TenantID,
		fieldValue:    beforeToolID,
	}, map[string]any{
		fieldTenantID: payload.TenantID,
		fieldValue:    payload.ToolID,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit mcp pass-through upstream update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleDisableXTenantKeyUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.DISABLE_X_TENANT_KEY.SET",
		"disable_x_tenant_key",
		d.Store.GetDisableXTenantKey,
		d.Store.SetDisableXTenantKey,
	)
}

func (d *Dependencies) handleFeatureRateLimitingUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.FEATURE_RATE_LIMITING.SET",
		"feature_rate_limiting",
		d.Store.GetFeatureRateLimiting,
		d.Store.SetFeatureRateLimiting,
	)
}

func (d *Dependencies) handleFeatureArgConstraintsUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.FEATURE_ARG_CONSTRAINTS.SET",
		"feature_arg_constraints",
		d.Store.GetFeatureArgConstraints,
		d.Store.SetFeatureArgConstraints,
	)
}

func (d *Dependencies) handleFeatureSessionTokensUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.FEATURE_SESSION_TOKENS.SET",
		"feature_session_tokens",
		d.Store.GetFeatureSessionTokens,
		d.Store.SetFeatureSessionTokens,
	)
}

func (d *Dependencies) handleFeatureFileGovernanceUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.FEATURE_FILE_GOVERNANCE.SET",
		"feature_file_governance",
		d.Store.GetFeatureFileGovernance,
		d.Store.SetFeatureFileGovernance,
	)
}

func (d *Dependencies) handleSessionTokenTTLUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload DefaultApprovalTTLRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	if payload.Seconds < 60 || payload.Seconds > 86400 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "seconds must be between 60 and 86400"})
	}

	beforeTTL, _ := d.Store.GetSessionTokenTTLSeconds(c.Request().Context())
	if err := d.Store.SetSessionTokenTTLSeconds(c.Request().Context(), payload.Seconds); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update session token ttl"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.SESSION_TOKEN_TTL.SET", "SETTINGS", "session_token_ttl_seconds", map[string]any{
		fieldValue: beforeTTL,
	}, map[string]any{
		fieldValue: payload.Seconds,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit session token ttl update",
			fieldDetail: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleSecretProviderAWSUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.SECRET_PROVIDER_AWS.SET",
		"secret_provider_aws",
		d.Store.GetSecretProviderAWS,
		d.Store.SetSecretProviderAWS,
	)
}

func (d *Dependencies) handleSecretProviderGCPUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.SECRET_PROVIDER_GCP.SET",
		"secret_provider_gcp",
		d.Store.GetSecretProviderGCP,
		d.Store.SetSecretProviderGCP,
	)
}

func (d *Dependencies) handleSecretProviderVaultUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.SECRET_PROVIDER_VAULT.SET",
		"secret_provider_vault",
		d.Store.GetSecretProviderVault,
		d.Store.SetSecretProviderVault,
	)
}

func (d *Dependencies) handleSecretProviderAzureUpdate(c *echo.Context) error {
	return d.handleBooleanSystemSettingUpdate(
		c,
		"SETTINGS.SECRET_PROVIDER_AZURE.SET",
		"secret_provider_azure",
		d.Store.GetSecretProviderAzure,
		d.Store.SetSecretProviderAzure,
	)
}

func (d *Dependencies) handleBooleanSystemSettingUpdate(
	c *echo.Context,
	auditAction string,
	resourceID string,
	getter func(context.Context) (bool, error),
	setter func(context.Context, bool) error,
) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}

	var payload BooleanSettingRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	beforeValue, _ := getter(c.Request().Context())
	if err := setter(c.Request().Context(), payload.Enabled); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update setting"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", auditAction, "SETTINGS", resourceID, map[string]any{
		fieldValue: beforeValue,
	}, map[string]any{
		fieldValue: payload.Enabled,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit setting update",
			fieldDetail: err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func isRateLimitScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "tenant", rateLimitScopeTenantAgent, rateLimitScopeTenantTool, rateLimitScopeTenantAgentTool:
		return true
	default:
		return false
	}
}

func (d *Dependencies) handleApprovalDecision(c *echo.Context, status, auditAction string) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeApprovalsDec)
	if err != nil {
		return err
	}

	var payload ApprovalDecisionRequest
	if decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload); decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	tenantID := c.Param(fieldTenantID)
	approvalID := c.Param(fieldApprovalRequestID)
	before, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errApprovalNotFound})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadApproval})
	}
	now := time.Now().UTC()
	if approvalExpired(&before, now) {
		if expireErr := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); expireErr == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval expired"})
		}
	}

	decidedAt := time.Now().UTC()
	switch approvalStatus(status) {
	case approvalStatusApproved:
		err = d.Store.ApproveApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case approvalStatusDenied:
		err = d.Store.DenyApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case approvalStatusRevoked:
		err = d.Store.RevokeApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid status"})
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errApprovalNotFound})
		}
		if errors.Is(err, store.ErrInvalidState) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval state invalid"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}

	after, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errFailedLoadApproval})
	}
	if d.Metrics != nil {
		switch approvalStatus(status) {
		case approvalStatusApproved:
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("approved").Inc()
		case approvalStatusDenied:
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("denied").Inc()
		case approvalStatusRevoked:
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("revoked").Inc()
		}
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, auditAction, "APPROVAL.REQUEST", approvalID, map[string]any{
		fieldStatus:        before.Status,
		"decided_at":       before.DecidedAt,
		"decided_by":       before.DecidedBy,
		"decision_comment": before.DecisionComment,
	}, map[string]any{
		fieldStatus:        after.Status,
		"decided_at":       after.DecidedAt,
		"decided_by":       after.DecidedBy,
		"decision_comment": after.DecisionComment,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit approval decision",
			fieldDetail: err.Error(),
		})
	}

	if d.TicketingService != nil {
		go d.TicketingService.OnApprovalDecided(context.Background(), tenantID, approvalID, status, adminKey.AdminKeyID, payload.Comment)
	}

	return c.JSON(http.StatusOK, after)
}

func (d *Dependencies) emitAuditEvent(c *echo.Context, adminKey models.AdminKey, tenantID, action, resourceType, resourceID string, before, after map[string]any) error {
	action = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, action)
	resourceType = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, resourceType)
	action = strings.ToUpper(strings.TrimSpace(action))
	resourceType = strings.ToUpper(strings.TrimSpace(resourceType))
	resourceID = strings.TrimSpace(resourceID)
	if action == "" || resourceType == "" {
		return errors.New("audit event action and resource type required")
	}
	if !regexp.MustCompile(`^[A-Z0-9_]+(\.[A-Z0-9_]+)*$`).MatchString(action) {
		return errors.New("audit event action violates format constraint")
	}
	beforeJSON, err := marshalAuditPayload(resourceType, before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalAuditPayload(resourceType, after)
	if err != nil {
		return err
	}
	event := models.AdminAuditEvent{
		AuditEventID: "ae_" + uuid.NewString(),
		TenantID:     tenantID,
		ActorType:    actorTypeAdminKey,
		ActorID:      adminKey.AdminKeyID,
		ActorDisplay: adminKey.AdminKeyID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       beforeJSON,
		After:        afterJSON,
		RequestID:    c.Request().Header.Get("X-Request-Id"),
		IP:           c.RealIP(),
		UserAgent:    c.Request().UserAgent(),
		CreatedAt:    time.Now().UTC(),
	}
	return d.Store.InsertAuditEvent(c.Request().Context(), &event)
}

func marshalAuditPayload(resourceType string, payload map[string]any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	redacted := audit.RedactPayload(resourceType, payload)
	data, err := audit.CanonicalJSON(redacted)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func isApprovalStatus(value string) bool {
	switch approvalStatus(value) {
	case approvalStatusPending,
		approvalStatusApproved,
		approvalStatusDenied,
		approvalStatusExecuting,
		approvalStatusExecuted,
		approvalStatusFailed,
		approvalStatusExpired,
		approvalStatusRevoked:
		return true
	default:
		return false
	}
}

func approvalExpired(approval *models.ApprovalRequest, now time.Time) bool {
	if approval == nil {
		return false
	}
	status := approvalStatus(approval.Status)
	if status != approvalStatusPending && status != approvalStatusApproved {
		return false
	}
	return now.After(approval.ExpiresAt)
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return 50, nil //nolint:mnd // ignore default limit.
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

func parseOffset(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

func parseExportLimit(value string) (int, error) {
	if value == "" {
		return 1000, nil //nolint:mnd // ignore default limit.
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

//nolint:nilnil // nil time with nil error indicates optional query parameter is absent.
func parseTimeParam(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}

func writeAuditCSV(writer *csv.Writer, events []models.AdminAuditEvent, includeDetails bool) error {
	header := []string{
		"audit_event_id",
		fieldTenantID,
		"stream_id",
		"event_hash",
		"prev_hash",
		"actor_type",
		"actor_id",
		"actor_display",
		"action",
		"resource_type",
		"resource_id",
		"request_id",
		"ip",
		"user_agent",
		"created_at",
	}
	if includeDetails {
		header = append(header, "before", "after")
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for i := range events {
		row := []string{
			events[i].AuditEventID,
			events[i].TenantID,
			events[i].StreamID,
			events[i].EventHash,
			events[i].PrevHash,
			events[i].ActorType,
			events[i].ActorID,
			events[i].ActorDisplay,
			events[i].Action,
			events[i].ResourceType,
			events[i].ResourceID,
			events[i].RequestID,
			events[i].IP,
			events[i].UserAgent,
			events[i].CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if includeDetails {
			row = append(row, string(events[i].Before), string(events[i].After))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func validateRegoModule(module string) error {
	if !strings.Contains(module, "import rego.v1") {
		return errors.New("rego module must import rego.v1")
	}
	if !strings.Contains(module, "package rbitr.policy") {
		return errors.New("rego module must define package rbitr.policy")
	}
	return nil
}

func isValidSecretRef(ref string) bool {
	return strings.HasPrefix(ref, "env://") || strings.HasPrefix(ref, "file://")
}
