package admin

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
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

func RegisterRoutes(e *echo.Echo, deps Dependencies) {
	adminGroup := e.Group("/admin")
	adminGroup.PUT("/tenants/:tenant_id/config", deps.handleTenantConfigUpdate)
	adminGroup.PUT("/tenants/:tenant_id/tools/:tool_id", deps.handleToolConfigUpdate)
	adminGroup.PUT("/tenants/:tenant_id/policy", deps.handlePolicyUpdate)
	adminGroup.PUT("/bootstrap/complete", deps.handleBootstrapComplete)
}

func (d Dependencies) handleTenantConfigUpdate(c *echo.Context) error {
	if err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
		return err
	}
	var payload TenantConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	if err := d.Store.UpdateTenantConfig(c.Request().Context(), tenantID, payload.Name, payload.TenantKey); err != nil {
		return handleBootstrapError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleToolConfigUpdate(c *echo.Context) error {
	if err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
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
	if err := d.Store.UpdateToolConfig(c.Request().Context(), tenantID, toolID, payload.BaseURL, payload.AuthType, payload.AuthValue); err != nil {
		return handleBootstrapError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handlePolicyUpdate(c *echo.Context) error {
	if err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
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
	if err := d.Store.UpdatePolicy(c.Request().Context(), tenantID, payload.RegoModule, payload.PolicyVersion); err != nil {
		return handleBootstrapError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleBootstrapComplete(c *echo.Context) error {
	if err := requireAdminScope(c, d.Store, "admin:write"); err != nil {
		return err
	}
	if err := d.Store.MarkBootstrapComplete(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "bootstrap completion failed"})
	}
	return c.NoContent(http.StatusNoContent)
}

func requireAdminScope(c *echo.Context, st store.StoreAPI, scope string) error {
	adminKey := c.Request().Header.Get(auth.AdminKeyHeader)
	if _, err := auth.AuthenticateAdmin(c.Request().Context(), st, adminKey, scope); err != nil {
		return authError(c, err)
	}
	return nil
}

func authError(c *echo.Context, err error) error {
	if err == auth.ErrUnauthorized {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	if err == auth.ErrForbidden {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "auth error"})
}

func handleBootstrapError(c *echo.Context, err error) error {
	if err == nil {
		return nil
	}
	if err == store.ErrBootstrapComplete {
		return c.JSON(http.StatusConflict, map[string]string{"error": "bootstrap already completed"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "update failed"})
}
