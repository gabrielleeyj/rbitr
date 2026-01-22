package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type Dependencies struct {
	Store   store.StoreAPI
	Metrics *telemetry.Metrics
	Config  config.Config
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

type PolicyUpdateRequest struct {
	RegoModule    string `json:"rego_module"`
	PolicyVersion string `json:"policy_version"`
}

type RiskOverrideRequest struct {
	ActionRisk string `json:"action_risk"`
}

func RegisterRoutes(e *echo.Echo, deps *Dependencies) {
	adminGroup := e.Group("/admin")
	adminGroup.PUT("/tenants/:tenant_id/config", deps.handleTenantConfigUpdate)
	adminGroup.PUT("/tenants/:tenant_id/tools/:tool_id", deps.handleToolConfigUpdate)
	adminGroup.PUT("/tenants/:tenant_id/policy", deps.handlePolicyUpdate)
	adminGroup.PUT("/tenants/:tenant_id/risk-overrides/:action_type", deps.handleRiskOverrideUpdate)
	adminGroup.GET("/tenants", deps.handleTenantList)
	adminGroup.GET("/tenants/:tenant_id", deps.handleTenantDetail)
	adminGroup.GET("/tenants/:tenant_id/evidence", deps.handleEvidenceList)
	adminGroup.GET("/tenants/:tenant_id/policies", deps.handlePolicyVersions)
	adminGroup.GET("/tenants/:tenant_id/policies/:policy_version", deps.handlePolicyVersionGet)
	adminGroup.POST("/tenants/:tenant_id/policies", deps.handlePolicyCreate)
	adminGroup.PUT("/tenants/:tenant_id/policies/:policy_version/publish", deps.handlePolicyPublish)
	adminGroup.PUT("/tenants/:tenant_id/policies/rollback", deps.handlePolicyRollback)
	adminGroup.POST("/tenants/:tenant_id/policies/simulate", deps.handlePolicySimulate)
	adminGroup.GET("/tenants/:tenant_id/risk-overrides", deps.handleRiskOverridesList)
	adminGroup.DELETE("/tenants/:tenant_id/risk-overrides/:action_type", deps.handleRiskOverrideDelete)
	adminGroup.GET("/tenants/:tenant_id/approvals", deps.handleApprovalsList)
	adminGroup.GET("/tenants/:tenant_id/approvals/:approval_request_id", deps.handleApprovalDetail)
	adminGroup.POST("/tenants/:tenant_id/approvals/:approval_request_id/approve", deps.handleApprovalApprove)
	adminGroup.POST("/tenants/:tenant_id/approvals/:approval_request_id/deny", deps.handleApprovalDeny)
	adminGroup.POST("/tenants/:tenant_id/approvals/:approval_request_id/revoke", deps.handleApprovalRevoke)
	adminGroup.GET("/tenants/:tenant_id/tools", deps.handleToolsList)
	adminGroup.GET("/tenants/:tenant_id/audit", deps.handleAuditList)
	adminGroup.GET("/audit", deps.handleAuditListAll)
	adminGroup.GET("/settings", deps.handleSettingsGet)
	adminGroup.PUT("/settings/default-approval-ttl", deps.handleDefaultApprovalTTLUpdate)
	adminGroup.PUT("/settings/admin-write-lock", deps.handleAdminWriteLock)
	adminGroup.PUT("/config/write-lock", deps.handleAdminWriteLock)
	adminGroup.PUT("/bootstrap/complete", deps.handleBootstrapComplete)
}

func (d Dependencies) handleTenantConfigUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
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
		afterKeyHash = utils.HashString(payload.TenantKey)
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

func (d Dependencies) handleToolConfigUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
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

func (d Dependencies) handlePolicyUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
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

func (d Dependencies) handleRiskOverrideUpdate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
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

func (d Dependencies) handleBootstrapComplete(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
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
	return key, nil
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
