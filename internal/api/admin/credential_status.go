package admin

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/credential"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

// CredentialStatusResponse is the response for the credential health check endpoint.
type CredentialStatusResponse struct {
	ToolID   string `json:"tool_id"`
	Provider string `json:"provider"`
	Healthy  bool   `json:"healthy"`
	Error    string `json:"error,omitempty"`
}

func (d *Dependencies) handleCredentialStatus(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeToolsRead); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param("tool_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, toolID)

	tool, err := d.Store.GetTool(c.Request().Context(), tenantID, toolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool"})
	}

	providerName := credential.ProviderName(tool.CredentialConfig)

	resp := CredentialStatusResponse{
		ToolID:   toolID,
		Provider: providerName,
		Healthy:  true,
	}

	if d.CredentialResolver == nil {
		// No resolver configured — fall back to basic static check.
		if tool.AuthType != authTypeNone && tool.AuthValue == "" && len(tool.CredentialConfig) == 0 {
			resp.Healthy = false
			resp.Error = "no credential configured"
		}
		return c.JSON(http.StatusOK, resp)
	}

	if healthErr := d.CredentialResolver.HealthCheck(c.Request().Context(), tool.AuthType, tool.AuthValue, tool.CredentialConfig); healthErr != nil {
		resp.Healthy = false
		resp.Error = healthErr.Error()
	}

	return c.JSON(http.StatusOK, resp)
}
