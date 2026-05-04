package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	oapkg "github.com/gabrielleeyj/rbitr/internal/openapi"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

// OpenAPIImportRequest is the request body for POST /admin/:tenant_id/tools/import/openapi.
type OpenAPIImportRequest struct {
	SpecURL         string           `json:"spec_url"`
	Mode            oapkg.ImportMode `json:"mode"`
	BaseURLOverride string           `json:"base_url_override"`
	AuthType        string           `json:"auth_type"`
	AuthValue       string           `json:"auth_value"`
	Prefix          string           `json:"prefix"`
}

// OpenAPIImportPreviewResponse is the response for the preview endpoint.
type OpenAPIImportPreviewResponse struct {
	Tools []oapkg.GeneratedTool `json:"tools"`
	Count int                   `json:"count"`
}

// OpenAPIImportConfirmRequest selects which tools from the preview to actually create.
type OpenAPIImportConfirmRequest struct {
	SpecURL         string           `json:"spec_url"`
	Mode            oapkg.ImportMode `json:"mode"`
	BaseURLOverride string           `json:"base_url_override"`
	AuthType        string           `json:"auth_type"`
	AuthValue       string           `json:"auth_value"`
	Prefix          string           `json:"prefix"`
	SelectedToolIDs []string         `json:"selected_tool_ids"`
}

func (d *Dependencies) handleOpenAPIImportPreview(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	if _, err := requireAdminScope(c, d.Store, scopeToolsWrite); err != nil {
		return err
	}

	var payload OpenAPIImportRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	if payload.SpecURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "spec_url is required"})
	}
	if payload.Mode == "" {
		payload.Mode = oapkg.ModeSingle
	}
	if payload.Mode != oapkg.ModeSingle && payload.Mode != oapkg.ModeMulti {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "mode must be 'single' or 'multi'"})
	}

	req := oapkg.ImportRequest{
		SpecURL:         payload.SpecURL,
		Mode:            payload.Mode,
		BaseURLOverride: payload.BaseURLOverride,
		AuthType:        payload.AuthType,
		AuthValue:       payload.AuthValue,
		Prefix:          payload.Prefix,
	}

	tools, err := oapkg.ParseAndGenerate(c.Request().Context(), &req)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, OpenAPIImportPreviewResponse{
		Tools: tools,
		Count: len(tools),
	})
}

func (d *Dependencies) handleOpenAPIImportConfirm(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeToolsWrite)
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	var payload OpenAPIImportConfirmRequest
	if decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload); decodeErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errInvalidRequestBody})
	}

	if payload.SpecURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "spec_url is required"})
	}
	if payload.Mode == "" {
		payload.Mode = oapkg.ModeSingle
	}

	req := oapkg.ImportRequest{
		SpecURL:         payload.SpecURL,
		Mode:            payload.Mode,
		BaseURLOverride: payload.BaseURLOverride,
		AuthType:        payload.AuthType,
		AuthValue:       payload.AuthValue,
		Prefix:          payload.Prefix,
	}

	generated, err := oapkg.ParseAndGenerate(c.Request().Context(), &req)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}

	// Filter to selected tools if specified.
	selected := filterSelected(generated, payload.SelectedToolIDs)
	if len(selected) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no tools selected for import"})
	}

	models := oapkg.ToModels(selected, tenantID, payload.AuthValue)

	var created []ToolResponse
	var skipped []string
	for i := range models {
		if err := d.Store.InsertTool(c.Request().Context(), &models[i]); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				skipped = append(skipped, models[i].ToolID)
				continue
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error":     "failed to insert tool",
				fieldDetail: models[i].ToolID,
			})
		}
		created = append(created, toolToResponse(&models[i]))
	}

	d.invalidateTenantCaches(tenantID)

	afterAudit := map[string]any{
		"spec_url":      payload.SpecURL,
		"mode":          payload.Mode,
		"created_count": len(created),
		"skipped":       skipped,
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TOOL.IMPORT_OPENAPI", "TOOL", payload.SpecURL, nil, afterAudit); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to audit import",
			fieldDetail: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"created": created,
		"skipped": skipped,
	})
}

func filterSelected(tools []oapkg.GeneratedTool, selectedIDs []string) []oapkg.GeneratedTool {
	if len(selectedIDs) == 0 {
		return tools
	}

	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = true
	}

	result := make([]oapkg.GeneratedTool, 0, len(selectedIDs))
	for i := range tools {
		if selected[tools[i].ToolID] {
			result = append(result, tools[i])
		}
	}
	return result
}
