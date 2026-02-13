package admin

import (
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
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/opa"
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
	PolicyVersion string         `json:"policy_version"`
	RegoModule    string         `json:"rego_module"`
	Input         map[string]any `json:"input"`
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
	NotifyApprovalExpiring     bool   `json:"notify_approval_expiring"`
	NotifyTokenAbuse           bool   `json:"notify_token_abuse"`
	NotifyPolicyInvalid        bool   `json:"notify_policy_invalid"`
}

type NotificationMetadataResponse struct {
	EventTypes []string `json:"event_types"`
	Severities []string `json:"severities"`
	Channels   []string `json:"channels"`
}

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
	AdminWriteLock            bool `json:"admin_write_lock"`
	DefaultApprovalTTLSeconds int  `json:"default_approval_ttl_seconds"`
	AuditRetentionDays        int  `json:"audit_retention_days"`
}

type HTTPConfig struct {
	BaseURL  string `json:"base_url"`
	AuthType string `json:"auth_type"`
	AuthSet  bool   `json:"auth_set"`
}

type MCPConfig struct {
	UpstreamURL     string          `json:"upstream_url"`
	Description     string          `json:"description"`
	InputSchemaJSON json.RawMessage `json:"input_schema_json"`
}

type ToolResponse struct {
	ToolID   string      `json:"tool_id"`
	TenantID string      `json:"tenant_id"`
	HTTP     *HTTPConfig `json:"http,omitempty"`
	MCP      *MCPConfig  `json:"mcp,omitempty"`
}

func (d Dependencies) handleTenantList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	items, err := d.Store.ListTenants(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tenants"})
	}
	return c.JSON(http.StatusOK, items)
}

func (d Dependencies) handleTenantDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	item, err := d.Store.GetTenant(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tenant not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d Dependencies) handleEvidenceList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	decision := c.QueryParam("decision")
	actionType := c.QueryParam("action_type")
	risk := c.QueryParam("risk")
	var since *time.Time
	if sinceParam := c.QueryParam("since"); sinceParam != "" {
		parsed, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
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
		record := &records[i]
		export := models.ActionDecisionExport{
			DecisionID:        record.DecisionID,
			RequestID:         record.RequestID,
			TenantID:          record.TenantID,
			AgentID:           record.AgentID,
			ToolID:            record.ToolID,
			ActionType:        record.ActionType,
			ActionRisk:        record.ActionRisk,
			ActionSummary:     record.ActionSummary,
			Decision:          record.Decision,
			DecisionVersion:   record.DecisionVersion,
			DecisionRisk:      record.DecisionRisk,
			RuleID:            record.RuleID,
			RulePriority:      record.RulePriority,
			Reasons:           record.Reasons,
			Constraints:       record.Constraints,
			Tags:              record.Tags,
			PolicyVersion:     record.PolicyVersion,
			Reason:            record.Reason,
			RequestHash:       record.RequestHash,
			ResponseHash:      record.ResponseHash,
			ApprovalRequestID: record.ApprovalRequestID,
			Timestamp:         record.CreatedAt,
		}
		if record.ApprovalRequestID != "" {
			if approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, record.ApprovalRequestID); err == nil {
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
	return c.JSON(http.StatusOK, map[string]any{"tenant_id": tenantID, "records": exported})
}

func (d Dependencies) handleApprovalsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	status := strings.ToUpper(strings.TrimSpace(c.QueryParam("status")))
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
				approvals[i].Status = "EXPIRED"
				approvals[i].DecidedAt = &now
			}
		}
	}
	return c.JSON(http.StatusOK, approvals)
}

func (d Dependencies) handleApprovalsPendingCount(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	count, err := d.Store.CountPendingApprovals(c.Request().Context(), tenantID, time.Now().UTC())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load pending approvals"})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"tenant_id":     tenantID,
		"pending_count": count,
	})
}

func (d Dependencies) handleApprovalDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	approvalID := c.Param("approval_request_id")
	approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	now := time.Now().UTC()
	if approvalExpired(&approval, now) {
		if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); err == nil {
			approval.Status = "EXPIRED"
			approval.DecidedAt = &now
		}
	}
	return c.JSON(http.StatusOK, approval)
}

func (d Dependencies) handleApprovalApprove(c *echo.Context) error {
	return d.handleApprovalDecision(c, "APPROVED", "APPROVAL.REQUEST.APPROVE")
}

func (d Dependencies) handleApprovalDeny(c *echo.Context) error {
	return d.handleApprovalDecision(c, "DENIED", "APPROVAL.REQUEST.DENY")
}

func (d Dependencies) handleApprovalRevoke(c *echo.Context) error {
	return d.handleApprovalDecision(c, "REVOKED", "APPROVAL.REQUEST.REVOKE")
}

func (d Dependencies) handlePolicyVersions(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	versions, err := d.Store.ListPolicyVersions(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list policies"})
	}
	config, err := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant config"})
	}

	response := PolicyVersionsResponse{
		TenantID:            tenantID,
		ActivePolicyVersion: config.ActivePolicyVersion,
		Versions:            versions,
	}
	return c.JSON(http.StatusOK, response)
}

func (d Dependencies) handlePolicyVersionGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	version := c.Param("policy_version")
	item, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load policy"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d Dependencies) handlePolicyCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload PolicyCreateRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.PolicyVersion == "" || payload.RegoModule == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "policy_version and rego_module required"})
	}
	if err := validateRegoModule(payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if _, err := opa.PrepareQuery(c.Request().Context(), payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":  "rego compilation failed",
			"detail": err.Error(),
		})
	}

	tenantID := c.Param("tenant_id")
	if err := d.Store.CreatePolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion, payload.RegoModule, adminKey.AdminKeyID, payload.Notes); err != nil {
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "admin writes locked"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to create policy",
			"detail": err.Error(),
		})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.CREATE", "POLICY.VERSION", payload.PolicyVersion, nil, map[string]any{
		"policy_version": payload.PolicyVersion,
		"created_by":     adminKey.AdminKeyID,
		"notes":          payload.Notes,
		"rego_sha256":    utils.HashString(payload.RegoModule),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy create",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusCreated)
}

func (d Dependencies) handlePolicyPublish(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	version := c.Param("policy_version")
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.Store.PublishPolicyVersion(c.Request().Context(), tenantID, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "admin writes locked"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to publish policy",
			"detail": err.Error(),
		})
	}
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", version, map[string]any{
		"active_policy_version": before.ActivePolicyVersion,
	}, map[string]any{
		"active_policy_version": after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy publish",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handlePolicyRollback(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload PolicyRollbackRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.Store.RollbackPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "admin writes locked"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to rollback policy",
			"detail": err.Error(),
		})
	}
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	target := payload.PolicyVersion
	if target == "" {
		target = after.ActivePolicyVersion
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.ROLLBACK", "POLICY.ACTIVE", target, map[string]any{
		"active_policy_version": before.ActivePolicyVersion,
	}, map[string]any{
		"active_policy_version": after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy rollback",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handlePolicySimulate(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	var payload PolicySimulationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Input == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "input required"})
	}

	tenantID := c.Param("tenant_id")
	regoModule := payload.RegoModule
	if regoModule == "" {
		if payload.PolicyVersion != "" {
			version, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion)
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
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
			"error":  "policy evaluation failed",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"decision": map[string]any{
			"version":     result.Version,
			"decision":    result.Decision,
			"risk":        result.Risk,
			"rule":        result.Rule,
			"reasons":     result.Reasons,
			"constraints": result.Constraints,
			"tags":        result.Tags,
		},
	})
}

func (d Dependencies) handleRiskOverridesList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	overrides, err := d.Store.ListRiskOverrides(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list overrides"})
	}
	return c.JSON(http.StatusOK, overrides)
}

func (d Dependencies) handleRiskOverrideDelete(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	actionType := c.Param("action_type")
	beforeRisk, _ := d.Store.GetRiskOverride(c.Request().Context(), tenantID, actionType)
	if err := d.Store.DeleteRiskOverride(c.Request().Context(), tenantID, actionType); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete override"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "RISK_OVERRIDE.DELETE", "RISK_OVERRIDE", actionType, map[string]any{
		"action_type": actionType,
		"action_risk": beforeRisk,
	}, nil); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit override delete",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleToolsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	tools, err := d.Store.ListTools(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tools"})
	}
	response := make([]ToolResponse, 0, len(tools))
	for _, tool := range tools {
		toolResponse := ToolResponse{
			ToolID:   tool.ToolID,
			TenantID: tool.TenantID,
		}

		// Add HTTP config if base_url is set
		if tool.BaseURL != "" {
			toolResponse.HTTP = &HTTPConfig{
				BaseURL:  tool.BaseURL,
				AuthType: tool.AuthType,
				AuthSet:  tool.AuthValue != "",
			}
		}

		// Add MCP config if any MCP fields are set
		if tool.MCPUpstreamURL != "" || tool.Description != "" || len(tool.InputSchemaJSON) > 0 {
			toolResponse.MCP = &MCPConfig{
				UpstreamURL:     tool.MCPUpstreamURL,
				Description:     tool.Description,
				InputSchemaJSON: tool.InputSchemaJSON,
			}
		}

		response = append(response, toolResponse)
	}
	return c.JSON(http.StatusOK, response)
}

func (d Dependencies) handleSettingsGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
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
	return c.JSON(http.StatusOK, SettingsResponse{
		AdminWriteLock:            locked,
		DefaultApprovalTTLSeconds: defaultTTL,
		AuditRetentionDays:        retentionDays,
	})
}

func (d Dependencies) handleAuditList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	tenantID := c.Param("tenant_id")
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from"})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to"})
	}
	events, err := d.Store.ListAuditEvents(c.Request().Context(), tenantID, limit, offset, action, resourceType, actorID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to list audit events",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d Dependencies) handleAuditResourceTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	values, err := d.Store.ListAuditResourceTypes(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list audit resource types"})
	}
	return c.JSON(http.StatusOK, map[string][]string{
		"resource_types": values,
	})
}

func (d Dependencies) handleAuditExport(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	return d.handleAuditExportResponse(c, tenantID)
}

func (d Dependencies) handleAuditExportResponse(c *echo.Context, tenantID string) error {
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
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
		}
		offset, err = parseOffset(c.QueryParam("offset"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
		}
	}
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from"})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to"})
	}
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

func (d Dependencies) handleAuditListAll(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	from, err := parseTimeParam(c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid from"})
	}
	to, err := parseTimeParam(c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid to"})
	}
	events, err := d.Store.ListAuditEvents(c.Request().Context(), "", limit, offset, action, resourceType, actorID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to list audit events",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d Dependencies) handleNotificationConfigGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "notification config not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load notification config"})
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
		NotifyApprovalExpiring:     config.NotifyApprovalExpiring,
		NotifyTokenAbuse:           config.NotifyTokenAbuse,
		NotifyPolicyInvalid:        config.NotifyPolicyInvalid,
		UpdatedAt:                  config.UpdatedAt,
	})
}

func (d Dependencies) handleNotificationConfigUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	var payload NotificationConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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

	tenantID := c.Param("tenant_id")
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
		NotifyApprovalExpiring:     payload.NotifyApprovalExpiring,
		NotifyTokenAbuse:           payload.NotifyTokenAbuse,
		NotifyPolicyInvalid:        payload.NotifyPolicyInvalid,
	}
	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update notification config"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.UPDATE", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		"slack_webhook_enabled": before.SlackWebhookEnabled,
		"slack_bot_enabled":     before.SlackBotEnabled,
		"email_enabled":         before.EmailEnabled,
		"email_provider":        before.EmailProvider,
	}, map[string]any{
		"slack_webhook_enabled": payload.SlackWebhookEnabled,
		"slack_bot_enabled":     payload.SlackBotEnabled,
		"email_enabled":         payload.EmailEnabled,
		"email_provider":        payload.EmailProvider,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit notification update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationSlackSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid secret_ref"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.SlackWebhookSecretRef != "" || before.SlackBotSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.SlackWebhookSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update slack secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.SLACK_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		"configured": beforeConfigured,
	}, map[string]any{
		"configured": true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit slack secret ref",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationEmailSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid secret_ref"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	beforeConfigured := before.EmailSecretRef != ""
	config := before
	config.TenantID = tenantID
	config.EmailSecretRef = payload.SecretRef

	if err := d.Store.UpsertNotificationConfig(c.Request().Context(), config); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update email secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.NOTIFICATIONS.EMAIL_SECRET_REF.SET", "TENANT.NOTIFICATIONS", tenantID, map[string]any{
		"configured": beforeConfigured,
	}, map[string]any{
		"configured": true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit email secret ref",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleMailingListsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	lists, err := d.Store.ListMailingLists(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list mailing lists"})
	}
	return c.JSON(http.StatusOK, lists)
}

func (d Dependencies) handleMailingListCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	var payload MailingListRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
	}
	for _, email := range payload.Members {
		if !strings.Contains(email, "@") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid member email"})
		}
	}

	tenantID := c.Param("tenant_id")
	list := models.MailingList{
		MailingListID: uuid.NewString(),
		TenantID:      tenantID,
		Name:          payload.Name,
		Description:   payload.Description,
	}
	if err := d.Store.CreateMailingList(c.Request().Context(), list, payload.Members); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.CREATE", "MAILING_LIST", list.MailingListID, map[string]any{}, map[string]any{
		"name":         payload.Name,
		"description":  payload.Description,
		"member_count": len(payload.Members),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit mailing list create",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, list)
}

func (d Dependencies) handleMailingListUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	var payload MailingListRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name required"})
	}
	for _, email := range payload.Members {
		if !strings.Contains(email, "@") {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid member email"})
		}
	}

	tenantID := c.Param("tenant_id")
	listID := c.Param("mailing_list_id")
	before, _ := d.Store.GetMailingList(c.Request().Context(), tenantID, listID)
	list := models.MailingList{
		MailingListID: listID,
		TenantID:      tenantID,
		Name:          payload.Name,
		Description:   payload.Description,
	}
	if err := d.Store.UpdateMailingList(c.Request().Context(), list, payload.Members); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.UPDATE", "MAILING_LIST", listID, map[string]any{
		"name":        before.Name,
		"description": before.Description,
	}, map[string]any{
		"name":         payload.Name,
		"description":  payload.Description,
		"member_count": len(payload.Members),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit mailing list update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleMailingListDelete(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	listID := c.Param("mailing_list_id")
	before, _ := d.Store.GetMailingList(c.Request().Context(), tenantID, listID)
	if err := d.Store.DeleteMailingList(c.Request().Context(), tenantID, listID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete mailing list"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.MAILING_LIST.DELETE", "MAILING_LIST", listID, map[string]any{
		"name":        before.Name,
		"description": before.Description,
	}, map[string]any{}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit mailing list delete",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationTestSlack(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "notifications not configured"})
	}
	tenantID := c.Param("tenant_id")
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "notification config missing"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load notification config"})
	}
	if !config.SlackWebhookEnabled || config.SlackWebhookSecretRef == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "slack webhook not configured"})
	}
	msg := notifications.NotificationMessage{
		Title:  "Slack notification test",
		Body:   "This is a test notification from rbitr.",
		Fields: map[string]string{"Tenant": tenantID},
	}
	if err := d.Notifications.Send(c.Request().Context(), tenantID, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  "NOTIFICATIONS.TEST",
		Severity:   "INFO",
		ResourceID: "test",
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send slack test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationTestSlackBot(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "notifications not configured"})
	}
	tenantID := c.Param("tenant_id")
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "notification config missing"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load notification config"})
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
		Body:   "This is a test notification from rbitr.",
		Fields: map[string]string{"Tenant": tenantID},
	}
	if err := engine.Send(c.Request().Context(), notifications.SlackBotChannel, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  "NOTIFICATIONS.TEST",
		Severity:   "INFO",
		ResourceID: "test",
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send slack bot test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationTestEmail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
		return err
	}
	if d.Notifications == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": "notifications not configured"})
	}
	tenantID := c.Param("tenant_id")
	config, err := d.Store.GetNotificationConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "notification config missing"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load notification config"})
	}
	if !config.EmailEnabled || config.EmailProvider == "" || config.EmailFrom == "" || config.EmailDefaultMailingListID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email not configured"})
	}
	msg := notifications.NotificationMessage{
		Title:  "Email notification test",
		Body:   "This is a test notification from rbitr.",
		Fields: map[string]string{"Tenant": tenantID},
	}
	if err := d.Notifications.Send(c.Request().Context(), tenantID, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  "NOTIFICATIONS.TEST",
		Severity:   "INFO",
		ResourceID: "test",
	}, msg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to send email test"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleNotificationSuppressions(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
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

func (d Dependencies) handleNotificationEventTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	channels := []string{
		notifications.SlackWebhookChannel,
		notifications.SlackBotChannel,
		notifications.EmailChannel,
	}
	sort.Strings(channels)
	return c.JSON(http.StatusOK, NotificationMetadataResponse{
		EventTypes: notifications.EventTypes(),
		Severities: notifications.Severities(),
		Channels:   channels,
	})
}

func (d Dependencies) handleActionTypes(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string][]string{
		"action_types": classification.ActionTypes(),
	})
}

func (d Dependencies) handleDefaultApprovalTTLUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload DefaultApprovalTTLRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if payload.Seconds < 60 || payload.Seconds > 86400 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "seconds must be between 60 and 86400"})
	}

	beforeTTL, _ := d.Store.GetDefaultApprovalTTLSeconds(c.Request().Context())
	if err := d.Store.SetDefaultApprovalTTLSeconds(c.Request().Context(), payload.Seconds); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update default approval ttl"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.APPROVAL_TTL_DEFAULT.SET", "SETTINGS", "default_approval_ttl_seconds", map[string]any{
		"value": beforeTTL,
	}, map[string]any{
		"value": payload.Seconds,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit approval ttl update",
			"detail": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleAuditRetentionUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload AuditRetentionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Days < 30 || payload.Days > 3650 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "days must be between 30 and 3650"})
	}
	beforeValue, _ := d.Store.GetAuditRetentionDays(c.Request().Context())
	if err := d.Store.SetAuditRetentionDays(c.Request().Context(), payload.Days); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update audit retention"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.AUDIT_RETENTION.SET", "SETTINGS", "audit_retention_days", map[string]any{
		"value": beforeValue,
	}, map[string]any{
		"value": payload.Days,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit retention update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleApprovalDecision(c *echo.Context, status, auditAction string) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload ApprovalDecisionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	approvalID := c.Param("approval_request_id")
	before, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	now := time.Now().UTC()
	if approvalExpired(&before, now) {
		if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); err == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval expired"})
		}
	}

	decidedAt := time.Now().UTC()
	switch status {
	case "APPROVED":
		err = d.Store.ApproveApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case "DENIED":
		err = d.Store.DenyApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case "REVOKED":
		err = d.Store.RevokeApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid status"})
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		if errors.Is(err, store.ErrInvalidState) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval state invalid"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}

	after, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	if d.Metrics != nil {
		switch status {
		case "APPROVED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("approved").Inc()
		case "DENIED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("denied").Inc()
		case "REVOKED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("revoked").Inc()
		}
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, auditAction, "APPROVAL.REQUEST", approvalID, map[string]any{
		"status":           before.Status,
		"decided_at":       before.DecidedAt,
		"decided_by":       before.DecidedBy,
		"decision_comment": before.DecisionComment,
	}, map[string]any{
		"status":           after.Status,
		"decided_at":       after.DecidedAt,
		"decided_by":       after.DecidedBy,
		"decision_comment": after.DecisionComment,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit approval decision",
			"detail": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, after)
}

func (d Dependencies) emitAuditEvent(c *echo.Context, adminKey models.AdminKey, tenantID, action, resourceType, resourceID string, before, after map[string]any) error {
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
		ActorType:    "admin_key",
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
	return d.Store.InsertAuditEvent(c.Request().Context(), event)
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
	switch value {
	case "PENDING", "APPROVED", "DENIED", "EXECUTED", "EXPIRED", "REVOKED":
		return true
	default:
		return false
	}
}

func approvalExpired(approval *models.ApprovalRequest, now time.Time) bool {
	if approval == nil {
		return false
	}
	if approval.Status != "PENDING" && approval.Status != "APPROVED" {
		return false
	}
	return now.After(approval.ExpiresAt)
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return 50, nil
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
		return 1000, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

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
		"tenant_id",
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
	for _, event := range events {
		row := []string{
			event.AuditEventID,
			event.TenantID,
			event.StreamID,
			event.EventHash,
			event.PrevHash,
			event.ActorType,
			event.ActorID,
			event.ActorDisplay,
			event.Action,
			event.ResourceType,
			event.ResourceID,
			event.RequestID,
			event.IP,
			event.UserAgent,
			event.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if includeDetails {
			row = append(row, string(event.Before), string(event.After))
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
