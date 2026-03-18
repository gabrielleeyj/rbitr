package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type Dependencies struct {
	Store         store.StoreAPI
	Notifications *notifications.Service
	Metrics       *telemetry.Metrics
	Config        config.Config
	ToolCache     *cache.TTLCache[models.Tool]
	RiskCache     *cache.TTLCache[string]
}

type TenantConfigRequest struct {
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
}

type ToolConfigRequest struct {
	BaseURL   string `json:"base_url"`
	AuthType  string `json:"auth_type"`
	AuthValue string `json:"auth_value"`
}

type ToolMetadataRequest struct {
	Description     string          `json:"description"`
	MCPUpstreamURL  string          `json:"mcp_upstream_url"`
	InputSchemaJSON json.RawMessage `json:"input_schema_json"`
}

type PolicyUpdateRequest struct {
	RegoModule    string `json:"rego_module"`
	PolicyVersion string `json:"policy_version"`
}

type RiskOverrideRequest struct {
	ActionRisk string `json:"action_risk"`
}

func RegisterRoutes(e *echo.Echo, deps *Dependencies) {
	adminGroup := e.Group("/admin")

	adminGroup.GET("/tenants", deps.handleTenantList)
	adminGroup.POST("/tenants", deps.handleTenantCreate)

	tenantGroup := adminGroup.Group("/tenants/:tenant_id", deps.requireTenantVisible)
	tenantGroup.GET("", deps.handleTenantDetail)
	tenantGroup.DELETE("", deps.handleTenantDelete)
	tenantGroup.GET("/evidence", deps.handleEvidenceList)
	tenantGroup.PUT("/config", deps.handleTenantConfigUpdate)
	tenantGroup.PUT("/tools/:tool_id", deps.handleToolConfigUpdate)
	tenantGroup.PUT("/policy", deps.handlePolicyUpdate)

	tenantGroup.GET("/policies", deps.handlePolicyVersions)
	tenantGroup.GET("/policies/:policy_version", deps.handlePolicyVersionGet)
	tenantGroup.POST("/policies", deps.handlePolicyCreate)
	tenantGroup.POST("/policies/simulate", deps.handlePolicySimulate)
	tenantGroup.PUT("/policies/:policy_version/publish", deps.handlePolicyPublish)
	tenantGroup.PUT("/policies/rollback", deps.handlePolicyRollback)

	tenantGroup.GET("/risk-overrides", deps.handleRiskOverridesList)
	tenantGroup.PUT("/risk-overrides/:action_type", deps.handleRiskOverrideUpdate)
	tenantGroup.DELETE("/risk-overrides/:action_type", deps.handleRiskOverrideDelete)

	tenantGroup.GET("/approvals", deps.handleApprovalsList)
	tenantGroup.GET("/approvals/:approval_request_id", deps.handleApprovalDetail)
	tenantGroup.GET("/approvals/pending-count", deps.handleApprovalsPendingCount)
	tenantGroup.POST("/approvals/:approval_request_id/approve", deps.handleApprovalApprove)
	tenantGroup.POST("/approvals/:approval_request_id/deny", deps.handleApprovalDeny)
	tenantGroup.POST("/approvals/:approval_request_id/revoke", deps.handleApprovalRevoke)

	tenantGroup.GET("/tools", deps.handleToolsList)
	tenantGroup.PUT("/tools/:tool_id/metadata", deps.handleToolMetadataUpdate)

	tenantGroup.GET("/audit", deps.handleAuditList)
	tenantGroup.GET("/audit/export", deps.handleAuditExport)
	tenantGroup.GET("/audit/resource-types", deps.handleAuditResourceTypes)

	tenantGroup.GET("/notifications", deps.handleNotificationConfigGet)
	tenantGroup.PUT("/notifications", deps.handleNotificationConfigUpdate)
	tenantGroup.PUT("/notifications/slack-secret-ref", deps.handleNotificationSlackSecretRefSet)
	tenantGroup.PUT("/notifications/email-secret-ref", deps.handleNotificationEmailSecretRefSet)
	tenantGroup.POST("/notifications/test/slack", deps.handleNotificationTestSlack)
	tenantGroup.POST("/notifications/test/slack-bot", deps.handleNotificationTestSlackBot)
	tenantGroup.POST("/notifications/test/email", deps.handleNotificationTestEmail)
	tenantGroup.GET("/notifications/suppressions", deps.handleNotificationSuppressions)

	tenantGroup.GET("/mailing-lists", deps.handleMailingListsList)
	tenantGroup.POST("/mailing-lists", deps.handleMailingListCreate)
	tenantGroup.PUT("/mailing-lists/:mailing_list_id", deps.handleMailingListUpdate)
	tenantGroup.DELETE("/mailing-lists/:mailing_list_id", deps.handleMailingListDelete)

	tenantGroup.PUT("/enabled", deps.handleTenantSetEnabled)
	tenantGroup.GET("/keys", deps.handleTenantKeysList)
	tenantGroup.POST("/keys", deps.handleTenantKeyCreate)
	tenantGroup.POST("/keys/rotate", deps.handleTenantKeyRotate)
	tenantGroup.POST("/keys/:key_id/revoke", deps.handleTenantKeyRevoke)

	adminGroup.GET("/me", deps.handleAdminMe)
	adminGroup.GET("/action-types", deps.handleActionTypes)
	adminGroup.GET("/audit", deps.handleAuditListAll)
	adminGroup.PUT("/bootstrap/complete", deps.handleBootstrapComplete)
	adminGroup.PUT("/config/write-lock", deps.handleAdminWriteLock)
	adminGroup.GET("/notifications/event-types", deps.handleNotificationEventTypes)
	adminGroup.GET("/settings", deps.handleSettingsGet)
	adminGroup.PUT("/settings/default-approval-ttl", deps.handleDefaultApprovalTTLUpdate)
	adminGroup.PUT("/settings/default-rate-limit", deps.handleDefaultRateLimitUpdate)
	adminGroup.PUT("/settings/audit-retention", deps.handleAuditRetentionUpdate)
	adminGroup.PUT("/settings/disable-x-tenant-key", deps.handleDisableXTenantKeyUpdate)
	adminGroup.PUT("/settings/feature-rate-limiting", deps.handleFeatureRateLimitingUpdate)
	adminGroup.PUT("/settings/feature-arg-constraints", deps.handleFeatureArgConstraintsUpdate)
	adminGroup.PUT("/settings/feature-session-tokens", deps.handleFeatureSessionTokensUpdate)
	adminGroup.PUT("/settings/feature-file-governance", deps.handleFeatureFileGovernanceUpdate)
	adminGroup.PUT("/settings/session-token-ttl", deps.handleSessionTokenTTLUpdate)
	adminGroup.PUT("/settings/enforcement-mode", deps.handleEnforcementModeUpdate)
	adminGroup.PUT("/settings/mcp-passthrough-upstream", deps.handleMCPPassthroughUpstreamUpdate)
	adminGroup.PUT("/settings/admin-write-lock", deps.handleAdminWriteLock)
}

func (d *Dependencies) requireTenantVisible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		adminKey := auth.AdminKeyFromRequest(c.Request())
		key, err := auth.AuthenticateAdminAny(c.Request().Context(), d.Store, adminKey)
		if err != nil {
			return authError(c, err)
		}
		if key.AdminKeyID != "" {
			c.Set(telemetry.CtxAdminID, key.AdminKeyID)
		}

		tenantID := c.Param("tenant_id")
		if tenantID == "" {
			return next(c)
		}
		if _, err := d.Store.GetTenant(c.Request().Context(), tenantID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "tenant not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
		}
		return next(c)
	}
}

func (d *Dependencies) handleTenantConfigUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeTenantsWrite)
	if err != nil {
		return err
	}
	var payload TenantConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	beforeName := ""
	beforeKeyHash := ""
	if tenant, err := d.Store.GetTenant(c.Request().Context(), tenantID); err == nil {
		beforeName = tenant.Name
	}
	if keyHash, err := d.Store.GetTenantKeyHash(c.Request().Context(), tenantID); err == nil {
		beforeKeyHash = keyHash
	}
	if err := d.Store.UpdateTenantConfig(c.Request().Context(), tenantID, payload.Name, payload.TenantKey); err != nil {
		return handleBootstrapError(c, err)
	}
	afterName := beforeName
	if payload.Name != "" {
		afterName = payload.Name
	}
	afterKeyHash := beforeKeyHash
	if payload.TenantKey != "" {
		afterKeyHash = utils.HashTenantKey(payload.TenantKey)
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.CONFIG.UPDATE", "TENANT.CONFIG", tenantID, map[string]any{
		"name":     beforeName,
		"key_hash": beforeKeyHash,
	}, map[string]any{
		"name":     afterName,
		"key_hash": afterKeyHash,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit tenant config update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleToolConfigUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}
	var payload ToolConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if payload.BaseURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "base_url required"})
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param("tool_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, toolID)
	beforeTool, _ := d.Store.GetTool(c.Request().Context(), tenantID, toolID)
	if err := d.Store.UpdateToolConfig(c.Request().Context(), tenantID, toolID, payload.BaseURL, payload.AuthType, payload.AuthValue); err != nil {
		return handleBootstrapError(c, err)
	}
	d.invalidateTenantCaches(tenantID)
	beforeAudit := map[string]any{
		"base_url":  beforeTool.BaseURL,
		"auth_type": beforeTool.AuthType,
		"auth_set":  beforeTool.AuthValue != "",
	}
	afterAudit := map[string]any{
		"base_url":  payload.BaseURL,
		"auth_type": payload.AuthType,
		"auth_set":  payload.AuthValue != "",
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.CONFIG.UPDATE", "TOOL", toolID, beforeAudit, afterAudit); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit tool update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleToolMetadataUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}

	var payload ToolMetadataRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate input schema JSON if provided
	// Note: JSON Schema allows boolean schemas (true/false) in addition to objects
	if len(payload.InputSchemaJSON) > 0 {
		var schemaTest any
		if err := json.Unmarshal(payload.InputSchemaJSON, &schemaTest); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "input_schema_json must be valid JSON"})
		}
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param("tool_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, toolID)

	// Get current tool state for audit
	beforeTool, _ := d.Store.GetTool(c.Request().Context(), tenantID, toolID)

	// Update tool metadata
	if err := d.Store.UpdateToolMetadata(
		c.Request().Context(),
		tenantID,
		toolID,
		payload.Description,
		payload.MCPUpstreamURL,
		payload.InputSchemaJSON,
	); err != nil {
		return handleBootstrapError(c, err)
	}
	d.invalidateTenantCaches(tenantID)

	// Emit audit event
	beforeAudit := map[string]any{
		"description":       beforeTool.Description,
		"mcp_upstream_url":  beforeTool.MCPUpstreamURL,
		"input_schema_json": beforeTool.InputSchemaJSON,
	}
	afterAudit := map[string]any{
		"description":       payload.Description,
		"mcp_upstream_url":  payload.MCPUpstreamURL,
		"input_schema_json": payload.InputSchemaJSON,
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.METADATA.UPDATE", "TOOL", toolID, beforeAudit, afterAudit); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit tool metadata update",
			"detail": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handlePolicyUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesWrite)
	if err != nil {
		return err
	}
	var payload PolicyUpdateRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.RegoModule == "" || payload.PolicyVersion == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "rego_module and policy_version required"})
	}

	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	beforeConfig, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.Store.UpdatePolicy(c.Request().Context(), tenantID, payload.RegoModule, payload.PolicyVersion); err != nil {
		return handleBootstrapError(c, err)
	}
	d.invalidateTenantCaches(tenantID)
	afterConfig, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", payload.PolicyVersion, map[string]any{
		"active_policy_version": beforeConfig.ActivePolicyVersion,
	}, map[string]any{
		"active_policy_version": afterConfig.ActivePolicyVersion,
		"rego_sha256":           utils.HashString(payload.RegoModule),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleRiskOverrideUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopePoliciesWrite)
	if err != nil {
		return err
	}

	var payload RiskOverrideRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !isValidRisk(payload.ActionRisk) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid action_risk"})
	}

	tenantID := c.Param("tenant_id")
	actionType := c.Param("action_type")
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxActionType, actionType)
	beforeRisk, _ := d.Store.GetRiskOverride(c.Request().Context(), tenantID, actionType)
	if err := d.Store.UpdateRiskOverride(c.Request().Context(), tenantID, actionType, payload.ActionRisk); err != nil {
		return handleBootstrapError(c, err)
	}
	d.invalidateTenantCaches(tenantID)
	if err := d.emitAuditEvent(c, adminKey, tenantID, "RISK_OVERRIDE.UPSERT", "RISK_OVERRIDE", actionType, map[string]any{
		"action_type": actionType,
		"action_risk": beforeRisk,
	}, map[string]any{
		"action_type": actionType,
		"action_risk": payload.ActionRisk,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit risk override update",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleBootstrapComplete(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeSettingsWrite)
	if err != nil {
		return err
	}
	before, _ := d.Store.GetBootstrapComplete(c.Request().Context())
	if err := d.Store.MarkBootstrapComplete(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "bootstrap completion failed"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "BOOTSTRAP.COMPLETE", "BOOTSTRAP", "bootstrap_complete", map[string]any{
		"bootstrap_complete": before,
	}, map[string]any{
		"bootstrap_complete": true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit bootstrap completion",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func requireAdminScope(c *echo.Context, st store.StoreAPI, scope string) (models.AdminKey, error) {
	adminKey := auth.AdminKeyFromRequest(c.Request())
	key, err := auth.AuthenticateAdmin(c.Request().Context(), st, adminKey, scope)
	if err != nil {
		_ = authError(c, err)
		return models.AdminKey{}, err
	}
	if key.AdminKeyID != "" {
		c.Set(telemetry.CtxAdminID, key.AdminKeyID)
	}
	return key, nil
}

func (d *Dependencies) handleAdminMe(c *echo.Context) error {
	adminKey := auth.AdminKeyFromRequest(c.Request())
	key, err := auth.AuthenticateAdminAny(c.Request().Context(), d.Store, adminKey)
	if err != nil {
		_ = authError(c, err)
		return err
	}
	if key.AdminKeyID != "" {
		c.Set(telemetry.CtxAdminID, key.AdminKeyID)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"admin_key_id": key.AdminKeyID,
		"scopes":       key.Scopes,
	})
}

func authError(c *echo.Context, err error) error {
	if errors.Is(err, auth.ErrUnauthorized) {
		c.Response().Header().Set("WWW-Authenticate", "Bearer")
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	if errors.Is(err, auth.ErrForbidden) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "auth error"})
}

func handleBootstrapError(c *echo.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrBootstrapComplete) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "bootstrap already completed"})
	}
	if errors.Is(err, store.ErrAdminWriteLocked) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "admin writes locked"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "update failed"})
}

func isValidRisk(value string) bool {
	switch value {
	case "LOW", "MEDIUM", "HIGH", "CRITICAL":
		return true
	default:
		return false
	}
}
