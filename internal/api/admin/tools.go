package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/credential"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

const transportHTTP = "http"

// CreateToolRequest is the request body for POST /admin/:tenant_id/tools.
type CreateToolRequest struct {
	ToolID           string          `json:"tool_id"`
	BaseURL          string          `json:"base_url"`
	AuthType         string          `json:"auth_type"`
	AuthValue        string          `json:"auth_value"`
	Description      string          `json:"description"`
	Transport        string          `json:"transport"`
	MCPUpstreamURL   string          `json:"mcp_upstream_url"`
	InputSchemaJSON  json.RawMessage `json:"input_schema_json"`
	CredentialConfig json.RawMessage `json:"credential_config,omitempty"`
}

func (d *Dependencies) handleToolCreate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}

	var payload CreateToolRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	if err := validateToolID(payload.ToolID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if payload.Transport == "" {
		payload.Transport = transportHTTP
	}
	if err := validateTransport(payload.Transport); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if payload.AuthType == "" {
		payload.AuthType = authTypeNone
	}
	if err := validateAuthType(payload.AuthType); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	switch payload.Transport {
	case transportHTTP:
		if payload.BaseURL == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "base_url required for http transport"})
		}
		if err := connector.ValidateToolBaseURL(payload.BaseURL, d.Config.OutboundAllowPrivate); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	case transportMCP:
		if payload.MCPUpstreamURL == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "mcp_upstream_url required for mcp transport"})
		}
		if err := connector.ValidateToolBaseURL(payload.MCPUpstreamURL, d.Config.OutboundAllowPrivate); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}

	if err := validateInputSchemaJSON(payload.InputSchemaJSON); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Validate credential_config if provided.
	if len(payload.CredentialConfig) > 0 {
		cfg, cfgErr := credential.ParseConfig(payload.CredentialConfig)
		if cfgErr != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": cfgErr.Error()})
		}
		if cfgErr = cfg.Validate(); cfgErr != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": cfgErr.Error()})
		}
	}

	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, payload.ToolID)

	tool := models.Tool{
		ToolID:           payload.ToolID,
		TenantID:         tenantID,
		BaseURL:          payload.BaseURL,
		AuthType:         payload.AuthType,
		AuthValue:        payload.AuthValue,
		Transport:        payload.Transport,
		MCPUpstreamURL:   payload.MCPUpstreamURL,
		Description:      payload.Description,
		InputSchemaJSON:  payload.InputSchemaJSON,
		CredentialConfig: payload.CredentialConfig,
	}

	if err := d.Store.InsertTool(c.Request().Context(), &tool); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "tool_id already exists for this tenant"})
		}
		return handleBootstrapError(c, err)
	}

	d.invalidateTenantCaches(tenantID)

	afterAudit := map[string]any{
		fieldToolID:             payload.ToolID,
		fieldBaseURL:            payload.BaseURL,
		"auth_type":             payload.AuthType,
		fieldAuthSet:            payload.AuthValue != "",
		"transport":             payload.Transport,
		fieldCredentialProvider: credential.ProviderName(payload.CredentialConfig),
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.CREATE", "TOOL", payload.ToolID, nil, afterAudit); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit tool creation",
			fieldDetail: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, toolToResponse(&tool))
}

func (d *Dependencies) handleToolGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeToolsRead); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param(fieldToolID)

	tool, err := d.Store.GetTool(c.Request().Context(), tenantID, toolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get tool"})
	}

	return c.JSON(http.StatusOK, toolToResponse(&tool))
}

func (d *Dependencies) handleToolArchive(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param(fieldToolID)
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, toolID)

	if err := d.Store.ArchiveTool(c.Request().Context(), tenantID, toolID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found or already archived"})
		}
		return handleBootstrapError(c, err)
	}

	d.invalidateTenantCaches(tenantID)

	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.ARCHIVE", "TOOL", toolID, map[string]any{fieldToolID: toolID}, nil); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit tool archive",
			fieldDetail: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleToolRestore(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	toolID := c.Param(fieldToolID)
	c.Set(telemetry.CtxTenantID, tenantID)
	c.Set(telemetry.CtxToolID, toolID)

	if err := d.Store.RestoreTool(c.Request().Context(), tenantID, toolID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found or not archived"})
		}
		return handleBootstrapError(c, err)
	}

	d.invalidateTenantCaches(tenantID)

	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.RESTORE", "TOOL", toolID, nil, map[string]any{fieldToolID: toolID}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit tool restore",
			fieldDetail: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// toolToResponse converts a models.Tool to a ToolResponse, reusing the same
// structure as handleToolsList in control_plane.go.
func toolToResponse(t *models.Tool) ToolResponse {
	resp := ToolResponse{
		ToolID:     t.ToolID,
		TenantID:   t.TenantID,
		ArchivedAt: t.ArchivedAt,
		Source:     t.Source,
	}
	if t.BaseURL != "" {
		resp.HTTP = &HTTPConfig{
			BaseURL:            t.BaseURL,
			AuthType:           t.AuthType,
			AuthSet:            t.AuthValue != "" || len(t.CredentialConfig) > 0,
			CredentialProvider: credential.ProviderName(t.CredentialConfig),
			CredentialConfig:   credential.RedactConfig(t.CredentialConfig),
		}
	}
	if t.MCPUpstreamURL != "" || t.Description != "" || len(t.InputSchemaJSON) > 0 {
		resp.MCP = &MCPConfig{
			UpstreamURL:     t.MCPUpstreamURL,
			Description:     t.Description,
			InputSchemaJSON: t.InputSchemaJSON,
		}
	}
	return resp
}
