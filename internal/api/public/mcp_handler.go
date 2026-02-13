package public

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

// handleMCP handles MCP Streamable HTTP requests (JSON-RPC 2.0).
func (d *Dependencies) handleMCP(c *echo.Context) error {
	start := time.Now()
	if d.Metrics != nil {
		d.Metrics.GatewayRequests.Inc()
	}

	// Extract tenant_id from path
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	// Generate or extract request ID for correlation
	requestID := c.Request().Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	c.Set(telemetry.CtxRequestID, requestID)
	c.Response().Header().Set("X-Request-Id", requestID)

	// Authenticate tenant using X-Tenant-Key header
	tenantKey := c.Request().Header.Get(auth.TenantKeyHeader)
	agentID := c.Request().Header.Get(auth.AgentIDHeader)

	tenant, err := auth.AuthenticateTenant(c.Request().Context(), d.Store, tenantKey, agentID)
	if err != nil {
		// Return JSON-RPC error for auth failures
		errObj := &mcp.ErrorObject{
			Code:    mcp.ErrorUnauthorized,
			Message: "authentication failed",
		}
		return writeJSONRPCError(c, nil, errObj)
	}

	// Verify tenant_id matches authenticated tenant
	if tenant.TenantID != tenantID {
		errObj := &mcp.ErrorObject{
			Code:    mcp.ErrorUnauthorized,
			Message: "tenant mismatch",
		}
		return writeJSONRPCError(c, nil, errObj)
	}

	c.Set(telemetry.CtxAgentID, agentID)

	// Read request body into buffer for ID extraction and validation
	// This allows us to preserve the request ID even when validation fails
	bodyBytes, bodyReadErr := io.ReadAll(io.LimitReader(c.Request().Body, mcp.MaxRequestSize+1))
	if bodyReadErr != nil {
		return writeJSONRPCError(c, nil, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "failed to read request body",
		})
	}

	// Check size limit
	if len(bodyBytes) > int(mcp.MaxRequestSize) {
		// Try to extract ID for better error correlation
		extractedID := mcp.ExtractRequestID(bodyBytes[:mcp.MaxRequestSize])
		return writeJSONRPCError(c, extractedID, &mcp.ErrorObject{
			Code:    mcp.ErrorInvalidRequest,
			Message: "request body too large",
		})
	}

	// Attempt to extract request ID before validation (for error correlation)
	extractedID := mcp.ExtractRequestID(bodyBytes)

	// Parse and validate JSON-RPC request
	req, err := mcp.ValidateAndParseRequest(bodyBytes, mcp.MaxRequestSize)
	if err != nil {
		// Validation returns ErrorObject directly - use extracted ID for correlation
		if errObj, ok := err.(*mcp.ErrorObject); ok {
			return writeJSONRPCError(c, extractedID, errObj)
		}
		// Fallback for unexpected errors
		return writeJSONRPCError(c, extractedID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "internal error",
		})
	}

	// Set MCP method in context for logging (no secrets)
	c.Set(telemetry.CtxMCPMethod, req.Method)

	// Check if this is a notification (null or missing ID)
	// Per JSON-RPC 2.0 spec, notifications must not receive a response
	isNotification := req.ID == nil || (req.ID != nil && req.ID.IsNull())

	// Route to method handlers
	resp, err := d.routeMCPMethod(c, tenant, agentID, req)
	if err != nil {
		// Internal routing error - only respond if not a notification
		if isNotification {
			return c.NoContent(http.StatusNoContent)
		}
		return writeJSONRPCError(c, req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "internal error",
		})
	}

	// Track metrics
	if d.Metrics != nil {
		duration := time.Since(start)
		_ = duration // TODO: Add MCP-specific metrics in future
	}

	// For notifications, don't send a response
	if isNotification {
		return c.NoContent(http.StatusNoContent)
	}

	// Write JSON-RPC response
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return c.JSON(http.StatusOK, resp)
}

// routeMCPMethod routes JSON-RPC method calls to appropriate handlers.
func (d *Dependencies) routeMCPMethod(c *echo.Context, tenant models.Tenant, agentID string, req *mcp.Request) (*mcp.Response, error) {
	switch req.Method {
	case mcp.MethodToolsList:
		// TODO: Implement in Story 2
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method)), nil

	case mcp.MethodToolsCall:
		// TODO: Implement in Story 3
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method)), nil

	default:
		// Unknown method
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method)), nil
	}
}

// writeJSONRPCError writes a JSON-RPC error response.
func writeJSONRPCError(c *echo.Context, id *mcp.RequestID, errObj *mcp.ErrorObject) error {
	resp := mcp.NewErrorResponse(id, errObj)
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return c.JSON(http.StatusOK, resp)
}

// formatRequestID formats a RequestID for logging (no sensitive data).
func formatRequestID(id *mcp.RequestID) string {
	if id == nil {
		return "nil"
	}
	if id.IsNull() {
		return "null"
	}
	if s := id.String(); s != nil {
		return fmt.Sprintf("string:%s", *s)
	}
	if n := id.Number(); n != nil {
		return fmt.Sprintf("number:%v", *n)
	}
	return "unknown"
}
