package public

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

//nolint:gochecknoglobals // stream limits are package-level knobs for tests and controlled defaults.
var (
	mcpStreamMaxDuration             = 5 * time.Minute
	mcpStreamHeartbeatInterval       = 15 * time.Second
	mcpStreamMaxBytes          int64 = 1 << 20 // 1 MiB
)

var errMCPStreamLimitExceeded = errors.New("mcp stream byte limit exceeded")

type sseWriteLimiter struct {
	writer  io.Writer
	limit   int64
	written int64
}

func (l *sseWriteLimiter) Write(payload []byte) error {
	if l.limit > 0 && l.written+int64(len(payload)) > l.limit {
		return errMCPStreamLimitExceeded
	}
	n, err := l.writer.Write(payload)
	l.written += int64(n)
	return err
}

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

	tenant, agentID, err := d.authenticateMCPRequest(c, tenantID)
	if err != nil {
		code := mcp.ErrorUnauthorized
		msg := "authentication failed"
		if errors.Is(err, auth.ErrInvalidAgentID) {
			code = mcp.ErrorInvalidParams
			msg = "invalid agent_id"
		}
		if errors.Is(err, auth.ErrSessionExpired) {
			msg = "session token expired"
		}
		if errors.Is(err, auth.ErrSessionIPMismatch) {
			msg = "session token IP mismatch"
		}
		return writeJSONRPCError(c, nil, &mcp.ErrorObject{Code: code, Message: msg})
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
		errObj := &mcp.ErrorObject{}
		if errors.As(err, &errObj) {
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
	// Per MCP/JSON-RPC, notifications omit the id field entirely.
	isNotification := req.ID == nil

	// Route to method handlers
	resp, err := d.routeMCPMethod(c, tenant, agentID, req)
	if err != nil {
		// Internal routing error - only respond if not a notification
		if isNotification {
			return c.NoContent(http.StatusAccepted)
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
		return c.NoContent(http.StatusAccepted)
	}

	// Write JSON-RPC response
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return c.JSON(http.StatusOK, resp)
}

// handleMCPStream handles GET /v1/mcp/:tenant_id for Streamable HTTP transport.
func (d *Dependencies) handleMCPStream(c *echo.Context) error {
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	requestID := c.Request().Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	c.Set(telemetry.CtxRequestID, requestID)
	c.Response().Header().Set("X-Request-Id", requestID)

	tenant, agentID, err := d.authenticateMCPRequest(c, tenantID)
	if err != nil {
		return c.NoContent(http.StatusUnauthorized)
	}
	_ = tenant // used for tenant verification in authenticateMCPRequest
	c.Set(telemetry.CtxAgentID, agentID)

	streamCtx, cancel := context.WithTimeout(c.Request().Context(), mcpStreamMaxDuration)
	defer cancel()

	// Streamable SSE response with bounded duration/bytes and heartbeats.
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	limiter := &sseWriteLimiter{
		writer: c.Response(),
		limit:  mcpStreamMaxBytes,
	}
	if err := limiter.Write([]byte(": connected\n\n")); err != nil {
		return err
	}
	flusher, ok := c.Response().(http.Flusher)
	if ok {
		flusher.Flush()
	}

	ticker := time.NewTicker(mcpStreamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-streamCtx.Done():
			if errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
				if err := limiter.Write([]byte("event: close\ndata: {\"reason\":\"max_duration_reached\"}\n\n")); err != nil &&
					!errors.Is(err, errMCPStreamLimitExceeded) {
					return err
				}
				if ok {
					flusher.Flush()
				}
			}
			return nil
		case <-ticker.C:
			if err := limiter.Write([]byte(": heartbeat\n\n")); err != nil {
				if errors.Is(err, errMCPStreamLimitExceeded) {
					_ = limiter.Write([]byte("event: close\ndata: {\"reason\":\"max_bytes_reached\"}\n\n"))
					if ok {
						flusher.Flush()
					}
					return nil
				}
				return err
			}
			if ok {
				flusher.Flush()
			}
		}
	}
}

// routeMCPMethod routes JSON-RPC method calls to appropriate handlers.
func (d *Dependencies) routeMCPMethod(c *echo.Context, tenant models.Tenant, agentID string, req *mcp.Request) (*mcp.Response, error) {
	switch req.Method {
	case mcp.MethodInitialize:
		return d.handleInitialize(c, tenant, agentID, req)

	case mcp.MethodNotificationsInitialized:
		return d.handleInitializedNotification(req)

	case mcp.MethodToolsList:
		return d.handleToolsList(c, tenant, req)

	case mcp.MethodToolsCall:
		return d.handleToolsCall(c, tenant, agentID, req)

	default:
		// Unknown method - attempt pass-through to upstream MCP server
		return d.handlePassThrough(c, tenant, req)
	}
}

// handlePassThrough forwards unknown MCP methods to the upstream MCP server
// without governance (no policy eval, no ADR, no approval workflow).
//
//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handlePassThrough(c *echo.Context, tenant models.Tenant, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	tenantConfig, err := d.Store.GetTenantConfig(ctx, tenant.TenantID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to load tenant config")), nil
	}
	configuredToolID := strings.TrimSpace(tenantConfig.MCPPassthroughUpstreamToolID)

	// List tenant tools for explicit upstream selection or fallback.
	tools, listErr := d.Store.ListTools(ctx, tenant.TenantID)
	if listErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to list tools")), nil
	}

	var upstreamURL string
	selectedToolID := ""
	usedFallback := false
	//nolint:nestif // Pass-through selection keeps configured and fallback upstream resolution in one place.
	if configuredToolID != "" {
		for i := range tools {
			if tools[i].ToolID != configuredToolID {
				continue
			}
			if tools[i].Transport != "mcp_streamable_http" || strings.TrimSpace(tools[i].MCPUpstreamURL) == "" {
				return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("configured pass-through upstream unavailable")), nil
			}
			selectedToolID = tools[i].ToolID
			upstreamURL = tools[i].MCPUpstreamURL
			break
		}
		if upstreamURL == "" {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("configured pass-through upstream unavailable")), nil
		}
	} else {
		for i := range tools {
			if tools[i].Transport == "mcp_streamable_http" && strings.TrimSpace(tools[i].MCPUpstreamURL) != "" {
				selectedToolID = tools[i].ToolID
				upstreamURL = tools[i].MCPUpstreamURL
				usedFallback = true
				break
			}
		}
	}

	if upstreamURL == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method)), nil
	}
	if selectedToolID != "" {
		c.Set(telemetry.CtxToolID, selectedToolID)
	}
	if usedFallback {
		if d.Metrics != nil && d.Metrics.MCPPassthroughFallbackTotal != nil {
			d.Metrics.MCPPassthroughFallbackTotal.Inc()
		}
		c.Logger().Warn("mcp pass-through upstream fallback used",
			"tenant_id", tenant.TenantID,
			"tool_id", selectedToolID,
			"method", req.Method,
			"request_id", c.Request().Header.Get("X-Request-Id"),
		)
	}

	// Forward request as-is to upstream
	mcpClient := connector.NewMCPClient(mcpClientTimeout)
	start := time.Now()
	upstreamResp, forwardErr := mcpClient.ForwardRequest(ctx, upstreamURL, req)
	latencyMs := time.Since(start).Milliseconds()

	if d.Metrics != nil {
		d.Metrics.ToolLatencyMs.Observe(float64(latencyMs))
	}

	if forwardErr != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "upstream request failed",
		}), nil
	}

	return upstreamResp, nil
}

//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handleInitialize(c *echo.Context, tenant models.Tenant, agentID string, req *mcp.Request) (*mcp.Response, error) {
	var params mcp.InitializeParams
	if len(req.Params) > 0 {
		if unmarshalErr := json.Unmarshal(req.Params, &params); unmarshalErr != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid initialize params")), nil
		}
	}
	if params.ProtocolVersion == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("protocolVersion required")), nil
	}
	if params.ProtocolVersion != mcp.ProtocolVersion20251125 {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInvalidParams,
			Message: "invalid params: unsupported protocolVersion",
			Data: mustMarshalJSON(map[string]any{
				"supported": []string{mcp.ProtocolVersion20251125},
			}),
		}), nil
	}

	capabilities := map[string]any{
		"tools": map[string]any{
			"listChanged": false,
		},
	}

	result := mcp.InitializeResult{
		ProtocolVersion: mcp.ProtocolVersion20251125,
		Capabilities:    capabilities,
		ServerInfo: mcp.Implementation{
			Name:    "rbitr-gateway",
			Version: "v1",
		},
	}

	// Issue ephemeral session token if feature is enabled.
	if d.SessionManager != nil && d.Config.FeatureSessionTokens {
		sourceIP := extractClientIP(c)
		token, claims, err := d.SessionManager.IssueToken(tenant.TenantID, agentID, sourceIP)
		if err != nil {
			c.Logger().Error("session token issue failed",
				"tenant_id", tenant.TenantID,
				"agent_id", agentID,
				"error", err,
			)
		} else {
			result.Capabilities["session"] = map[string]any{
				"token":      token,
				"expires_at": claims.ExpiresAt,
				"session_id": claims.SessionID,
			}
		}
	}

	return mcp.NewSuccessResponse(req.ID, result)
}

//nolint:nilnil // notifications intentionally produce no JSON-RPC response.
func (d *Dependencies) handleInitializedNotification(req *mcp.Request) (*mcp.Response, error) {
	if req.ID != nil {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInvalidRequest,
			Message: "notifications/initialized must not include id",
		}), nil
	}
	return nil, nil
}

// handleToolsList handles the tools/list MCP method.
//
//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handleToolsList(c *echo.Context, tenant models.Tenant, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	// List all tools for the tenant
	tools, listErr := d.Store.ListTools(ctx, tenant.TenantID)
	if listErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to list tools")), nil
	}

	// Convert rbitr tools to MCP tool format
	mcpTools := make([]mcp.Tool, 0, len(tools))
	for i := range tools {
		mcpTool := mcp.Tool{
			Name:        tools[i].ToolID, // Use tool_id as the stable MCP tool name
			Description: tools[i].Description,
			InputSchema: tools[i].InputSchemaJSON,
		}

		// If no description is set, provide a default
		if mcpTool.Description == "" {
			mcpTool.Description = "No description available"
		}

		// If no input schema is set, provide a permissive default
		if len(mcpTool.InputSchema) == 0 {
			mcpTool.InputSchema = []byte(`{"type":"object","additionalProperties":true}`)
		}

		mcpTools = append(mcpTools, mcpTool)
	}

	// Create result
	result := mcp.ToolsListResult{
		Tools: mcpTools,
	}

	return mcp.NewSuccessResponse(req.ID, result)
}

// writeJSONRPCError writes a JSON-RPC error response.
func writeJSONRPCError(c *echo.Context, id *mcp.RequestID, errObj *mcp.ErrorObject) error {
	resp := mcp.NewErrorResponse(id, errObj)
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return c.JSON(http.StatusOK, resp)
}

// handleToolsCall handles the tools/call MCP method with full governance.
//
//nolint:nilerr,nilnil // JSON-RPC notifications/errors are represented via response payloads.
func (d *Dependencies) handleToolsCall(c *echo.Context, tenant models.Tenant, agentID string, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	// Check if this is a notification (missing ID)
	// Per MCP/JSON-RPC, notifications must not have side effects or responses.
	isNotification := req.ID == nil
	if isNotification {
		// For notifications, return immediately without executing
		// (The caller already returns 204 for notifications, so this is defensive)
		return nil, nil
	}

	// Parse MCP tools/call params
	var params mcp.ToolsCallParams
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid params")), nil
	}
	if unmarshalErr := json.Unmarshal(paramsBytes, &params); unmarshalErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid params structure")), nil
	}

	// Validate required fields
	if params.Name == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("tool name required")), nil
	}

	// Map MCP tool name to rbitr tool_id (currently 1:1)
	toolID := params.Name

	// Validate tool name BEFORE DB lookup to prevent abuse
	// Only allow alphanumeric, underscore, hyphen, and dot
	if !isValidToolID(toolID) {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid tool name format")), nil
	}

	// Verify tool exists before any side effects (governance, approvals, ADRs)
	tool, err := d.getToolCached(ctx, tenant.TenantID, toolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("tool not found: "+toolID)), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to lookup tool")), nil
	}

	// Set context for telemetry
	c.Set(telemetry.CtxToolID, toolID)

	// Tool exists and is valid, note it for potential upstream forwarding
	_ = tool // Will be used in Story 4 for upstream forwarding

	// Parse arguments as a map to extract approval token
	var argumentsMap map[string]any
	if len(params.Arguments) > 0 {
		if parseArgsErr := json.Unmarshal(params.Arguments, &argumentsMap); parseArgsErr != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid arguments format")), nil
		}
	} else {
		argumentsMap = make(map[string]any)
	}

	// Extract approval token if present (for approval resubmission)
	approvalToken := ""
	if tokenVal, ok := argumentsMap["_rbitr_approval_token"]; ok {
		tokenStr, ok := tokenVal.(string)
		if !ok || strings.TrimSpace(tokenStr) == "" {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("_rbitr_approval_token must be a non-empty string")), nil
		}
		approvalToken = tokenStr
		// Remove internal control fields from arguments before hashing/policy input.
		delete(argumentsMap, "_rbitr_approval_token")
	}

	// Extract approval request ID if present (required for approved resubmission).
	approvalRequestID := ""
	if approvalIDVal, ok := argumentsMap["_rbitr_approval_request_id"]; ok {
		approvalIDStr, ok := approvalIDVal.(string)
		if !ok || strings.TrimSpace(approvalIDStr) == "" {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("_rbitr_approval_request_id must be a non-empty string")), nil
		}
		approvalRequestID = approvalIDStr
		delete(argumentsMap, "_rbitr_approval_request_id")
	}
	if approvalToken != "" && approvalRequestID == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("_rbitr_approval_request_id is required when _rbitr_approval_token is provided")), nil
	}

	// Generate request ID for tracking (with safe type assertion)
	requestID, ok := c.Get(telemetry.CtxRequestID).(string)
	if !ok || requestID == "" {
		requestID = uuid.NewString()
		c.Set(telemetry.CtxRequestID, requestID)
	}

	// Convert arguments back to JSON for hashing (after removing approval token)
	argumentsJSON, err := json.Marshal(argumentsMap)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to serialize arguments")), nil
	}

	// Build sanitized request for upstream forwarding (never include internal control fields).
	forwardReq, err := buildForwardToolsCallRequest(req, params.Name, argumentsJSON)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to build upstream request")), nil
	}

	// Compute canonical request hash
	bodyHash := utils.HashBody(argumentsJSON)
	canonical := utils.CanonicalRequest{
		TenantID: tenant.TenantID,
		AgentID:  agentID,
		ToolID:   toolID,
		Method:   "MCP_CALL",
		Path:     "/tools/call",
		Headers:  map[string]string{},
		BodyHash: bodyHash,
	}
	requestHash := utils.HashCanonical(&canonical)

	// Classify the action (for MCP, we use the tool name and arguments structure)
	// For now, use a simple classification - in future, you could inspect arguments
	classificationResult := classification.Result{
		ActionType:    "MCP." + toolID,
		ActionRisk:    "MEDIUM", // Default risk, can be overridden by policy
		ActionSummary: "MCP tool call: " + toolID,
	}
	c.Set(telemetry.CtxActionType, classificationResult.ActionType)

	// Handle approval resubmission if token is present
	if approvalToken != "" {
		return d.handleMCPApprovedCall(c, tenant, agentID, toolID, requestID, requestHash, classificationResult, approvalToken, approvalRequestID, forwardReq)
	}
	enforcementMode, enforcementErr := d.tenantEnforcementMode(ctx, tenant.TenantID)
	if enforcementErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to load tenant enforcement mode")), nil
	}

	// Check for risk overrides
	if overrideRisk, lookupErr := d.getRiskOverrideCached(ctx, tenant.TenantID, classificationResult.ActionType); lookupErr == nil {
		classificationResult.ActionRisk = overrideRisk
	} else if !errors.Is(lookupErr, store.ErrNotFound) {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("risk override lookup failed")), nil
	}

	// Evaluate policy
	if d.Policy == nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("policy evaluator not configured")), nil
	}

	policyInput := map[string]any{
		"tenant_id":      tenant.TenantID,
		"agent_id":       agentID,
		"tool_id":        toolID,
		"action_type":    classificationResult.ActionType,
		"action_risk":    classificationResult.ActionRisk,
		"policy_version": "",
		"mcp":            true,
		"arguments":      argumentsMap,
	}

	// File access governance: detect file paths in arguments and block traversal/sandbox violations.
	if d.featureFileGovernanceEnabled(ctx) {
		if violation := d.checkFileAccess(argumentsMap, tenant.TenantID); violation != "" {
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorFileAccessDenied,
				Message: violation,
			}), nil
		}
	}

	decisionResult, err := d.Policy.Evaluate(ctx, tenant.TenantID, policyInput)
	if err != nil {
		if invalidReason, policyVersion, ok := policyInvalidReason(err); ok {
			c.Logger().Error("policy output invalid",
				"error", err,
				"tenant_id", tenant.TenantID,
				"agent_id", agentID,
				"tool_id", toolID,
				"policy_version", policyVersion,
				"request_id", requestID,
			)
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorPolicyInvalid,
				Message: "policy evaluation error",
				Data: mustMarshalJSON(map[string]any{
					"reason_code":    invalidReason,
					"policy_version": policyVersion,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("policy evaluation failed")), nil
	}

	// Set default decision if empty
	if decisionResult.Decision == "" {
		decisionResult.Decision = string(decisionDeny)
		decisionResult.Rule.ID = "rule_default_deny"
		decisionResult.Reasons = []models.DecisionReason{{Code: "DEFAULT_DENY", Message: "Default deny"}}
	}
	decisionResult.Constraints = withMatchedRulesConstraint(decisionResult.Constraints, decisionResult.MatchedRules)

	rateLimitViolation, err := d.enforceRateLimit(
		ctx,
		tenant.TenantID,
		agentID,
		toolID,
		classificationResult.ActionType,
		decisionResult.Constraints,
	)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("rate limit check failed")), nil
	}
	if rateLimitViolation != nil {
		c.Set(telemetry.CtxDecision, decisionDeny)

		decisionID := "d_" + uuid.NewString()
		reasons := []models.DecisionReason{{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "Rate limit exceeded",
		}}
		adr := models.ActionDecisionRecord{
			DecisionID:      decisionID,
			RequestID:       requestID,
			TenantID:        tenant.TenantID,
			AgentID:         agentID,
			ToolID:          toolID,
			ActionType:      classificationResult.ActionType,
			ActionRisk:      classificationResult.ActionRisk,
			ActionSummary:   classificationResult.ActionSummary,
			Decision:        string(decisionDeny),
			DecisionVersion: decisionResult.Version,
			DecisionRisk:    decisionResult.Risk,
			RuleID:          "rate_limit_" + rateLimitViolation.Window,
			RulePriority:    rulePriority,
			Reasons:         reasons,
			Constraints:     withRateLimitConstraint(decisionResult.Constraints, rateLimitViolation),
			Tags:            decisionResult.Tags,
			PolicyVersion:   decisionResult.PolicyVersion,
			Reason:          firstReasonMessage(reasons),
			RequestHash:     requestHash,
			CreatedAt:       time.Now().UTC(),
		}
		if err := d.Store.InsertADR(ctx, &adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(string(decisionDeny), classificationResult.ActionType).Inc()
		}
		if isShadowMode(enforcementMode) {
			return d.executeMCPShadowDeny(
				ctx,
				&tool,
				forwardReq,
				buildShadowDecisionMetadata("rate_limit_"+rateLimitViolation.Window, decisionResult.Risk, decisionResult.PolicyVersion, reasons, adr.Constraints),
			)
		}
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorRateLimitExceeded,
			Message: "rate limit exceeded",
			Data: mustMarshalJSON(map[string]any{
				"window":              rateLimitViolation.Window,
				"limit":               rateLimitViolation.Limit,
				"remaining":           rateLimitViolation.Remaining,
				"retry_after_seconds": rateLimitViolation.RetryAfterSeconds,
				"scope":               rateLimitViolation.Scope,
			}),
		}), nil
	}

	argConstraintViolation := d.enforceArgumentConstraints(ctx, decisionResult.Constraints, argumentsMap)
	if argConstraintViolation != nil {
		c.Set(telemetry.CtxDecision, decisionDeny)

		decisionID := "d_" + uuid.NewString()
		reasons := []models.DecisionReason{{
			Code:    argConstraintViolation.ReasonCode,
			Message: argConstraintViolation.Message,
		}}
		adr := models.ActionDecisionRecord{
			DecisionID:      decisionID,
			RequestID:       requestID,
			TenantID:        tenant.TenantID,
			AgentID:         agentID,
			ToolID:          toolID,
			ActionType:      classificationResult.ActionType,
			ActionRisk:      classificationResult.ActionRisk,
			ActionSummary:   classificationResult.ActionSummary,
			Decision:        string(decisionDeny),
			DecisionVersion: decisionResult.Version,
			DecisionRisk:    decisionResult.Risk,
			RuleID:          argConstraintRuleID(argConstraintViolation),
			RulePriority:    rulePriority,
			Reasons:         reasons,
			Constraints:     withArgConstraintFailures(decisionResult.Constraints, argConstraintViolation),
			Tags:            decisionResult.Tags,
			PolicyVersion:   decisionResult.PolicyVersion,
			Reason:          firstReasonMessage(reasons),
			RequestHash:     requestHash,
			CreatedAt:       time.Now().UTC(),
		}
		if err := d.Store.InsertADR(ctx, &adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(string(decisionDeny), classificationResult.ActionType).Inc()
		}
		if isShadowMode(enforcementMode) {
			return d.executeMCPShadowDeny(
				ctx,
				&tool,
				forwardReq,
				buildShadowDecisionMetadata(argConstraintRuleID(argConstraintViolation), decisionResult.Risk, decisionResult.PolicyVersion, reasons, adr.Constraints),
			)
		}
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "denied by policy",
			Data: mustMarshalJSON(map[string]any{
				"denied":              true,
				"policy_version":      decisionResult.PolicyVersion,
				"rule_id":             argConstraintRuleID(argConstraintViolation),
				"risk":                decisionResult.Risk,
				"reasons":             formatReasons(reasons),
				"constraint_failures": argConstraintFailuresAsMaps(argConstraintViolation.Failures),
			}),
		}), nil
	}

	c.Set(telemetry.CtxDecision, decisionResult.Decision)

	// Create Action Decision Record
	decisionID := "d_" + uuid.NewString()
	adr := models.ActionDecisionRecord{
		DecisionID:      decisionID,
		RequestID:       requestID,
		TenantID:        tenant.TenantID,
		AgentID:         agentID,
		ToolID:          toolID,
		ActionType:      classificationResult.ActionType,
		ActionRisk:      classificationResult.ActionRisk,
		ActionSummary:   classificationResult.ActionSummary,
		Decision:        decisionResult.Decision,
		DecisionVersion: decisionResult.Version,
		DecisionRisk:    decisionResult.Risk,
		RuleID:          decisionResult.Rule.ID,
		RulePriority:    decisionResult.Rule.Priority,
		Reasons:         decisionResult.Reasons,
		Constraints:     decisionResult.Constraints,
		Tags:            decisionResult.Tags,
		PolicyVersion:   decisionResult.PolicyVersion,
		Reason:          firstReasonMessage(decisionResult.Reasons),
		RequestHash:     requestHash,
		CreatedAt:       time.Now().UTC(),
	}

	// Handle different decision outcomes
	switch decisionResult.Decision {
	case string(decisionDeny):
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(string(decisionDeny), classificationResult.ActionType).Inc()
		}
		if err := d.Store.InsertADR(ctx, &adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}
		if isShadowMode(enforcementMode) {
			return d.executeMCPShadowDeny(
				ctx,
				&tool,
				forwardReq,
				buildShadowDecisionMetadata(decisionResult.Rule.ID, decisionResult.Risk, decisionResult.PolicyVersion, decisionResult.Reasons, decisionResult.Constraints),
			)
		}

		// Return JSON-RPC error for DENY
		denyData, _ := json.Marshal(map[string]any{
			"denied":         true,
			"policy_version": decisionResult.PolicyVersion,
			"rule_id":        decisionResult.Rule.ID,
			"risk":           decisionResult.Risk,
			"reasons":        formatReasons(decisionResult.Reasons),
			"matched_rules":  matchedRulesFromConstraints(decisionResult.Constraints),
		})
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "denied by policy",
			Data:    denyData,
		}), nil

	case "REQUIRE_APPROVAL":
		now := time.Now().UTC()
		token, err := generateApprovalToken()
		if err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to generate approval token")), nil
		}

		actionRisk := decisionResult.Risk
		if actionRisk == "" {
			actionRisk = classificationResult.ActionRisk
		}

		expiresAt := now.Add(approvalTTL(ctx, d.Store, decisionResult.Constraints))
		approvalID := "ar_" + uuid.NewString()

		approval := models.ApprovalRequest{
			ApprovalRequestID: approvalID,
			TenantID:          tenant.TenantID,
			AgentID:           agentID,
			ToolID:            toolID,
			ActionType:        classificationResult.ActionType,
			RequestHash:       requestHash,
			RequestContext:    buildMCPApprovalRequestContext(toolID, argumentsMap),
			Status:            "PENDING",
			ApprovalTokenHash: utils.HashString(token),
			ExpiresAt:         expiresAt,
			CreatedAt:         now,
			PolicyVersion:     decisionResult.PolicyVersion,
			RequestDecisionID: decisionID,
			ActionSummary:     classificationResult.ActionSummary,
			Risk:              actionRisk,
			RuleID:            decisionResult.Rule.ID,
			Reasons:           decisionResult.Reasons,
		}

		adr.ApprovalRequestID = approvalID

		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues("REQUIRE_APPROVAL", classificationResult.ActionType).Inc()
			d.Metrics.ApprovalsCreatedTotal.Inc()
		}

		if err := d.Store.InsertApprovalRequest(ctx, &approval); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist approval")), nil
		}
		if err := d.Store.InsertADR(ctx, &adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}

		// Return JSON-RPC error for REQUIRE_APPROVAL
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorApprovalRequired,
			Message: "approval required",
			Data: mustMarshalJSON(map[string]any{
				"approval_required":   true,
				"approval_request_id": approvalID,
				"approval_token":      token,
				"expires_at":          expiresAt.Format(time.RFC3339),
				"policy_version":      decisionResult.PolicyVersion,
				"rule_id":             decisionResult.Rule.ID,
				"risk":                actionRisk,
				"reasons":             formatReasons(decisionResult.Reasons),
				"instructions":        "Include the approval_token in arguments._rbitr_approval_token when resubmitting",
			}),
		}), nil

	case string(decisionAllow):
		// Forward to upstream MCP server
		if tool.MCPUpstreamURL == "" {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorInternalError,
				Message: "tool not configured for MCP",
				Data: mustMarshalJSON(map[string]any{
					"tool_id": toolID,
				}),
			}), nil
		}

		// Create MCP client and forward request
		mcpClient := connector.NewMCPClient(mcpClientTimeout)
		toolStart := time.Now()
		upstreamResp, err := mcpClient.ForwardRequest(ctx, tool.MCPUpstreamURL, forwardReq)
		toolLatencyMs := time.Since(toolStart).Milliseconds()

		if d.Metrics != nil {
			d.Metrics.ToolLatencyMs.Observe(float64(toolLatencyMs))
		}

		if err != nil {
			// Update ADR with error (no response hash)
			if storeErr := d.Store.InsertADR(ctx, &adr); storeErr != nil {
				c.Logger().Error("failed to persist ADR after upstream error", "error", storeErr)
			}

			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorInternalError,
				Message: "upstream tool execution failed",
				Data: mustMarshalJSON(map[string]any{
					"reason": "upstream_error",
				}),
			}), nil
		}

		// Compute response hash for audit trail
		responseBytes, _ := json.Marshal(upstreamResp.Result)
		responseHash := utils.HashBody(responseBytes)
		adr.ResponseHash = responseHash

		// Persist ADR with response hash
		if err := d.Store.InsertADR(ctx, &adr); err != nil {
			c.Logger().Error("failed to persist ADR after successful execution", "error", err)
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}

		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(string(decisionAllow), classificationResult.ActionType).Inc()
			d.Metrics.ToolExecTotal.Inc()
		}

		// Return upstream response directly
		return upstreamResp, nil

	default:
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "unknown decision",
			Data: mustMarshalJSON(map[string]any{
				"decision": decisionResult.Decision,
			}),
		}), nil
	}
}

// handleMCPApprovedCall handles MCP tool calls with approval tokens.
//
//nolint:nilerr // JSON-RPC errors are encoded in response payloads rather than Go errors.
func (d *Dependencies) handleMCPApprovedCall(c *echo.Context, tenant models.Tenant, agentID, toolID, requestID, requestHash string, classificationResult classification.Result, approvalToken, approvalRequestID string, forwardReq *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	if approvalRequestID == "" {
		return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInvalidParamsError("_rbitr_approval_request_id is required when _rbitr_approval_token is provided")), nil
	}

	approval, err := d.Store.GetApprovalForExecution(ctx, tenant.TenantID, approvalRequestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval not found",
				Data: mustMarshalJSON(map[string]any{
					"reason":              "approval_not_found",
					"approval_request_id": approvalRequestID,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("failed to lookup approval")), nil
	}

	tokenHash := utils.HashString(approvalToken)
	// Validate approval token.
	if !utils.SecureCompare(tokenHash, approval.ApprovalTokenHash) {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("token_invalid").Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval token invalid",
			Data: mustMarshalJSON(map[string]any{
				"reason":              "token_invalid",
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	// Verify request hash matches to prevent token reuse across different requests
	if approval.RequestHash != requestHash {
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval request hash mismatch",
			Data: mustMarshalJSON(map[string]any{
				"reason":              "hash_mismatch",
				"approval_request_id": approvalRequestID,
			}),
		}), nil
	}

	// Validate approval state
	now := time.Now().UTC()
	if approval.Status == "EXPIRED" || now.After(approval.ExpiresAt) {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("expired").Inc()
		}
		_ = d.Store.MarkApprovalExpired(ctx, tenant.TenantID, approval.ApprovalRequestID, now)
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval expired",
			Data: mustMarshalJSON(map[string]any{
				"reason":              "expired",
				"approval_request_id": approval.ApprovalRequestID,
				"expired_at":          approval.ExpiresAt.Format(time.RFC3339),
			}),
		}), nil
	}

	switch approval.Status {
	case "APPROVED":
		//nolint:nestif // Approval claim flow handles race states and retries in one transactional branch.
		if claimErr := d.Store.ClaimApprovalExecution(ctx, tenant.TenantID, approval.ApprovalRequestID, tokenHash, requestHash, now); claimErr != nil {
			if errors.Is(claimErr, store.ErrNotFound) {
				return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
					Code:    mcp.ErrorDeniedByPolicy,
					Message: "approval not found",
					Data: mustMarshalJSON(map[string]any{
						"reason":              "approval_not_found",
						"approval_request_id": approvalRequestID,
					}),
				}), nil
			}
			if errors.Is(claimErr, store.ErrInvalidState) {
				latest, stateErr := d.Store.GetApprovalForExecution(ctx, tenant.TenantID, approvalRequestID)
				if stateErr != nil {
					return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("failed to refresh approval state")), nil
				}
				if latest.Status == string(decisionExecuting) {
					if latest.ExecutedAt != nil {
						if d.Metrics != nil {
							d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
						}
						return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
							Code:    mcp.ErrorDeniedByPolicy,
							Message: "approval already executed",
							Data: mustMarshalJSON(map[string]any{
								"reason":              "approval_already_executed",
								"approval_request_id": latest.ApprovalRequestID,
							}),
						}), nil
					}
					if !approvalRetryAllowed(&latest, now) {
						if d.Metrics != nil {
							d.Metrics.ApprovalsExecuteTotal.WithLabelValues("retry_window_exceeded").Inc()
						}
						return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
							Code:    mcp.ErrorDeniedByPolicy,
							Message: "approval retry window exceeded",
							Data: mustMarshalJSON(map[string]any{
								"reason":               "executing_retry_window_exceeded",
								"approval_request_id":  latest.ApprovalRequestID,
								"retry_window_seconds": int(approvalExecutionRetryWindow.Seconds()),
							}),
						}), nil
					}
					approval = latest
				} else {
					if d.Metrics != nil {
						d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_claimed").Inc()
					}
					return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
						Code:    mcp.ErrorDeniedByPolicy,
						Message: "approval already claimed",
						Data: mustMarshalJSON(map[string]any{
							"reason":              "already_claimed",
							"status":              latest.Status,
							"approval_request_id": latest.ApprovalRequestID,
						}),
					}), nil
				}
			} else {
				return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("failed to claim approval")), nil
			}
		} else {
			approval.Status = string(decisionExecuting)
			approval.ExecutingAt = &now
			if approval.ExecutionID == "" {
				approval.ExecutionID = approval.ApprovalRequestID
			}
		}
	case string(decisionExecuting):
		if approval.ExecutedAt != nil {
			if d.Metrics != nil {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
			}
			return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval already executed",
				Data: mustMarshalJSON(map[string]any{
					"reason":              "approval_already_executed",
					"approval_request_id": approval.ApprovalRequestID,
				}),
			}), nil
		}
		if !approvalRetryAllowed(&approval, now) {
			if d.Metrics != nil {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("retry_window_exceeded").Inc()
			}
			return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval retry window exceeded",
				Data: mustMarshalJSON(map[string]any{
					"reason":               "executing_retry_window_exceeded",
					"approval_request_id":  approval.ApprovalRequestID,
					"retry_window_seconds": int(approvalExecutionRetryWindow.Seconds()),
				}),
			}), nil
		}
	case string(decisionExecuted):
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval already claimed",
			Data: mustMarshalJSON(map[string]any{
				"reason":              "already_claimed",
				"status":              approval.Status,
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	default:
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_claimed").Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval already claimed",
			Data: mustMarshalJSON(map[string]any{
				"reason":              "already_claimed",
				"status":              approval.Status,
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	// Set context for telemetry
	c.Set(telemetry.CtxActionType, approval.ActionType)
	c.Set(telemetry.CtxDecision, string(decisionAllow))

	decisionID := "d_" + uuid.NewString()
	ruleID := approval.RuleID
	if ruleID == "" {
		ruleID = "approval_granted"
	}
	policyVersion := approval.PolicyVersion
	if policyVersion == "" {
		policyVersion = "unknown"
	}

	actionType := approval.ActionType
	if actionType == "" {
		actionType = classificationResult.ActionType
	}
	actionRisk := approval.Risk
	if actionRisk == "" {
		actionRisk = classificationResult.ActionRisk
	}
	actionSummary := approval.ActionSummary
	if actionSummary == "" {
		actionSummary = classificationResult.ActionSummary
	}

	reasons := []models.DecisionReason{{Code: "APPROVED", Message: "Approved execution"}}

	// Get tool configuration for upstream forwarding
	tool, err := d.getToolCached(ctx, tenant.TenantID, toolID)
	if err != nil {
		_ = d.Store.MarkApprovalExecutionFailed(ctx, tenant.TenantID, approval.ApprovalRequestID, "UPSTREAM_ERROR", now)
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorInternalError,
				Message: "tool not found",
				Data: mustMarshalJSON(map[string]any{
					"tool_id":             toolID,
					"approval_request_id": approval.ApprovalRequestID,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("failed to lookup tool")), nil
	}

	// Check if tool is configured for MCP upstream
	if tool.MCPUpstreamURL == "" {
		_ = d.Store.MarkApprovalExecutionFailed(ctx, tenant.TenantID, approval.ApprovalRequestID, "UPSTREAM_ERROR", now)
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "tool not configured for MCP",
			Data: mustMarshalJSON(map[string]any{
				"tool_id":             tool.ToolID,
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	// Execute tool call via upstream MCP server
	mcpClient := connector.NewMCPClient(mcpClientTimeout)
	toolStart := time.Now()
	toolLatencyMs := time.Since(toolStart).Milliseconds()
	upstreamResp, err := mcpClient.ForwardRequest(ctx, tool.MCPUpstreamURL, forwardReq)

	if d.Metrics != nil {
		d.Metrics.ToolLatencyMs.Observe(float64(toolLatencyMs))
	}

	// Handle upstream execution error
	//nolint:nestif // Upstream error mapping keeps timeout/generic handling and metrics together.
	if err != nil {
		errorCode := "UPSTREAM_ERROR"
		reason := "upstream_error"
		if isUpstreamTimeoutError(err) {
			errorCode = string(decisionUpstreamTimeout)
			reason = "upstream_timeout"
		}
		_ = d.Store.MarkApprovalExecutionFailed(ctx, tenant.TenantID, approval.ApprovalRequestID, errorCode, time.Now().UTC())
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			if errorCode == string(decisionUpstreamTimeout) {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("upstream_timeout").Inc()
			} else {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("upstream_failed").Inc()
			}
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "upstream tool execution failed",
			Data: mustMarshalJSON(map[string]any{
				"reason":              reason,
				"approval_request_id": approval.ApprovalRequestID,
				"execution_id":        approvalExecutionID(&approval),
			}),
		}), nil
	}

	executedAt := time.Now().UTC()
	if err := d.Store.MarkApprovalExecuted(ctx, tenant.TenantID, approval.ApprovalRequestID, requestID, decisionID, executedAt); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("mark_executed_failed").Inc()
		}
		if errors.Is(err, store.ErrInvalidState) {
			return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval already claimed",
				Data: mustMarshalJSON(map[string]any{
					"reason":              "already_claimed",
					"approval_request_id": approval.ApprovalRequestID,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("failed to mark approval executed")), nil
	}

	// Compute response hash for audit trail.
	responseHash := ""
	if upstreamResp.Result != nil {
		responseBytes, _ := json.Marshal(upstreamResp.Result)
		responseHash = utils.HashBody(responseBytes)
	}

	adr := models.ActionDecisionRecord{
		DecisionID:        decisionID,
		RequestID:         requestID,
		TenantID:          tenant.TenantID,
		AgentID:           agentID,
		ToolID:            toolID,
		ActionType:        actionType,
		ActionRisk:        actionRisk,
		ActionSummary:     actionSummary,
		Decision:          "ALLOW",
		DecisionVersion:   "approval_execution",
		DecisionRisk:      actionRisk,
		RuleID:            ruleID,
		RulePriority:      0,
		Reasons:           reasons,
		Constraints:       map[string]any{},
		Tags:              nil,
		PolicyVersion:     policyVersion,
		Reason:            firstReasonMessage(reasons),
		RequestHash:       requestHash,
		ResponseHash:      responseHash,
		ApprovalRequestID: approval.ApprovalRequestID,
		CreatedAt:         executedAt,
	}
	if err := d.Store.InsertADR(ctx, &adr); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("adr_failed").Inc()
		}
		c.Logger().Error("failed to persist ADR after approval execution",
			"error", err,
			"approval_request_id", approval.ApprovalRequestID,
			"decision_id", decisionID,
		)
	}

	if d.Metrics != nil {
		d.Metrics.ApprovalsExecuteTotal.WithLabelValues("success").Inc()
		d.Metrics.DecisionsTotal.WithLabelValues(string(decisionAllow), actionType).Inc()
		d.Metrics.ToolExecTotal.Inc()
	}

	// Return upstream response directly
	return upstreamResp, nil
}

func approvalExecutionID(approval *models.ApprovalRequest) string {
	if approval == nil {
		return ""
	}
	if approval.ExecutionID != "" {
		return approval.ExecutionID
	}
	return approval.ApprovalRequestID
}

func buildMCPApprovalRequestContext(toolID string, arguments map[string]any) map[string]any {
	copiedArgs := make(map[string]any, len(arguments))
	for key, value := range arguments {
		copiedArgs[key] = value
	}
	return map[string]any{
		"transport": "mcp",
		"method":    "tools/call",
		"tool_name": toolID,
		"arguments": copiedArgs,
	}
}

func buildForwardToolsCallRequest(req *mcp.Request, toolName string, argumentsJSON []byte) (*mcp.Request, error) {
	params := mcp.ToolsCallParams{
		Name:      toolName,
		Arguments: json.RawMessage(argumentsJSON),
	}
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &mcp.Request{
		JSONRPC: req.JSONRPC,
		ID:      req.ID,
		Method:  req.Method,
		Params:  json.RawMessage(paramsBytes),
	}, nil
}

// formatReasons formats decision reasons for MCP error responses.
func formatReasons(reasons []models.DecisionReason) []string {
	result := make([]string, 0, len(reasons))
	for _, r := range reasons {
		result = append(result, r.Message)
	}
	return result
}

// mustMarshalJSON marshals a value to JSON, panicking on error.
// Used for known-good data structures in error responses.
func mustMarshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		// This should never happen with our controlled data structures
		return json.RawMessage(`{"error":"failed to marshal data"}`)
	}
	return data
}

// isValidToolID validates a tool ID for safe use in metrics labels.
// Allows alphanumeric, underscore, hyphen, and dot.
func isValidToolID(toolID string) bool {
	if toolID == "" || len(toolID) > 64 {
		return false
	}
	for _, r := range toolID {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// authenticateMCPRequest attempts session token auth first, then falls back to tenant key auth.
// Session tokens are only accepted when the feature is enabled and a SessionManager is configured.
func (d *Dependencies) authenticateMCPRequest(c *echo.Context, tenantID string) (models.Tenant, string, error) {
	// Try session token auth first if feature is enabled.
	if d.SessionManager != nil && d.Config.FeatureSessionTokens {
		if tenant, agentID, ok, err := d.authenticateViaSession(c, tenantID); ok || err != nil {
			return tenant, agentID, err
		}
	}

	// Fall back to tenant key auth.
	tenant, agentID, err := d.authenticateTenantRequest(c)
	if err != nil {
		return models.Tenant{}, "", err
	}

	// Verify tenant_id matches authenticated tenant.
	if tenant.TenantID != tenantID {
		return models.Tenant{}, "", auth.ErrUnauthorized
	}

	return tenant, agentID, nil
}

// authenticateViaSession validates a session token from the request.
// Returns (tenant, agentID, true, nil) on success, (_, _, false, nil) if no token present,
// or (_, _, false, err) on validation failure.
func (d *Dependencies) authenticateViaSession(c *echo.Context, tenantID string) (tenant models.Tenant, agentID string, ok bool, err error) {
	sessionToken := auth.SessionTokenFromRequest(c.Request())
	if sessionToken == "" {
		return models.Tenant{}, "", false, nil
	}

	sourceIP := extractClientIP(c)
	claims, err := d.SessionManager.ValidateToken(sessionToken, sourceIP)
	if err != nil {
		return models.Tenant{}, "", false, err
	}

	// Verify the session's tenant matches the path tenant.
	if claims.TenantID != tenantID {
		return models.Tenant{}, "", false, auth.ErrUnauthorized
	}

	// Verify the tenant still exists.
	summary, lookupErr := d.Store.GetTenant(c.Request().Context(), claims.TenantID)
	if lookupErr != nil {
		return models.Tenant{}, "", false, auth.ErrUnauthorized
	}

	return models.Tenant{
		TenantID: summary.TenantID,
		Name:     summary.Name,
		Enabled:  true, // existence in store implies enabled for session scope
	}, claims.AgentID, true, nil
}

// extractClientIP returns the client IP address from the echo context.
func extractClientIP(c *echo.Context) string {
	return strings.TrimSpace(c.RealIP())
}
