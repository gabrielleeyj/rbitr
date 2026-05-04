package public

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
)

const (
	resourceURIPrefix     = "rbitr://tools/"
	resourceURISuffix     = "/openapi"
	specFetchTimeout      = 10 * time.Second
	maxSpecBodyBytes      = 2 << 20 // 2 MiB
	sourceOpenAPIImport   = "openapi_import"
	transportHTTP         = "http"
	transportHTTPAPI      = "http_api"
	defaultInputSchemaStr = `{"type":"object","additionalProperties":true}`
)

// enrichToolSchema injects x-rbitr-endpoints metadata into the inputSchema for
// manually created HTTP tools that lack a rich schema. Tools imported from OpenAPI
// already have detailed schemas and are returned as-is.
func enrichToolSchema(tool *models.Tool, schema json.RawMessage) json.RawMessage {
	// Only enrich HTTP transport tools that weren't imported from OpenAPI.
	if tool.Source == sourceOpenAPIImport {
		return schema
	}
	if tool.Transport != transportHTTP && tool.Transport != transportHTTPAPI {
		return schema
	}

	// Parse existing schema to check if it already has meaningful content.
	var schemaMap map[string]any
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		return schema
	}

	// If schema already has x-rbitr-endpoints, don't overwrite.
	if _, ok := schemaMap["x-rbitr-endpoints"]; ok {
		return schema
	}

	// Add discovery hint so agents know this is an HTTP tool with a base URL.
	schemaMap["x-rbitr-endpoints"] = map[string]any{
		fieldBaseURL:  tool.BaseURL,
		"description": "HTTP tool — use path and method arguments to call specific endpoints",
	}

	enriched, err := json.Marshal(schemaMap)
	if err != nil {
		return schema
	}
	return enriched
}

// buildResourceURI creates a resource URI for a tool's OpenAPI spec.
func buildResourceURI(toolID string) string {
	return resourceURIPrefix + toolID + resourceURISuffix
}

// parseResourceURI extracts the tool ID from a resource URI.
// Returns the tool ID and true if the URI matches the expected pattern.
func parseResourceURI(uri string) (string, bool) {
	if !strings.HasPrefix(uri, resourceURIPrefix) || !strings.HasSuffix(uri, resourceURISuffix) {
		return "", false
	}
	toolID := strings.TrimPrefix(uri, resourceURIPrefix)
	toolID = strings.TrimSuffix(toolID, resourceURISuffix)
	if toolID == "" {
		return "", false
	}
	return toolID, true
}

// handleResourcesList returns MCP resources for tools with OpenAPI specs.
//
//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handleResourcesList(c *echo.Context, tenant models.Tenant, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	tools, listErr := d.Store.ListTools(ctx, tenant.TenantID, false, !d.Config.DevAutoTools)
	if listErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to list tools")), nil
	}

	resources := make([]mcp.Resource, 0)
	for i := range tools {
		if tools[i].OpenAPISpecURL == "" {
			continue
		}
		resources = append(resources, mcp.Resource{
			URI:         buildResourceURI(tools[i].ToolID),
			Name:        tools[i].ToolID + " OpenAPI spec",
			Description: "OpenAPI specification for " + tools[i].ToolID,
			MIMEType:    mimeApplicationJSON,
		})
	}

	return mcp.NewSuccessResponse(req.ID, mcp.ResourcesListResult{
		Resources: resources,
	})
}

// handleResourcesRead fetches and returns an OpenAPI spec for a tool.
//
//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handleResourcesRead(c *echo.Context, tenant models.Tenant, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	var params mcp.ResourcesReadParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid resources/read params")), nil
		}
	}
	if params.URI == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("uri is required")), nil
	}

	toolID, ok := parseResourceURI(params.URI)
	if !ok {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid resource URI format")), nil
	}

	tool, err := d.Store.GetTool(ctx, tenant.TenantID, toolID)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInvalidParams,
			Message: "resource not found",
		}), nil
	}

	if tool.OpenAPISpecURL == "" {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInvalidParams,
			Message: "no OpenAPI spec available for this tool",
		}), nil
	}

	specContent, fetchErr := fetchOpenAPISpec(ctx, tool.OpenAPISpecURL)
	if fetchErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to fetch OpenAPI spec")), nil
	}

	return mcp.NewSuccessResponse(req.ID, mcp.ResourcesReadResult{
		Contents: []mcp.ResourceContent{
			{
				URI:      params.URI,
				MIMEType: mimeApplicationJSON,
				Text:     specContent,
			},
		},
	})
}

// fetchOpenAPISpec fetches an OpenAPI spec from a URL.
func fetchOpenAPISpec(ctx context.Context, specURL string) (string, error) {
	client := &http.Client{Timeout: specFetchTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", mimeApplicationJSON)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch spec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spec endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBodyBytes))
	if err != nil {
		return "", fmt.Errorf("read spec body: %w", err)
	}

	return string(body), nil
}
