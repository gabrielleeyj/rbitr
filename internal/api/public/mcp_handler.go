package public

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
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
		return d.handleToolsList(c, tenant, req)

	case mcp.MethodToolsCall:
		return d.handleToolsCall(c, tenant, agentID, req)

	default:
		// Unknown method
		return mcp.NewErrorResponse(req.ID, mcp.NewMethodNotFoundError(req.Method)), nil
	}
}

// handleToolsList handles the tools/list MCP method.
func (d *Dependencies) handleToolsList(c *echo.Context, tenant models.Tenant, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	// List all tools for the tenant
	tools, err := d.Store.ListTools(ctx, tenant.TenantID)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to list tools")), nil
	}

	// Convert rbitr tools to MCP tool format
	mcpTools := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		mcpTool := mcp.Tool{
			Name:        tool.ToolID, // Use tool_id as the stable MCP tool name
			Description: tool.Description,
			InputSchema: tool.InputSchemaJSON,
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
func (d *Dependencies) handleToolsCall(c *echo.Context, tenant models.Tenant, agentID string, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	// Check if this is a notification (null or missing ID)
	// Per JSON-RPC 2.0 spec, notifications must not have side effects or responses
	isNotification := req.ID == nil || (req.ID != nil && req.ID.IsNull())
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
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
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
	tool, err := d.Store.GetTool(ctx, tenant.TenantID, toolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError(fmt.Sprintf("tool not found: %s", toolID))), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to lookup tool")), nil
	}

	// Set context for telemetry
	c.Set(telemetry.CtxToolID, toolID)

	// Tool exists and is valid, note it for potential upstream forwarding
	_ = tool // Will be used in Story 4 for upstream forwarding

	// Parse arguments as a map to extract approval token
	var argumentsMap map[string]interface{}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &argumentsMap); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("invalid arguments format")), nil
		}
	} else {
		argumentsMap = make(map[string]interface{})
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
		ActionType:    fmt.Sprintf("MCP.%s", toolID),
		ActionRisk:    "MEDIUM", // Default risk, can be overridden by policy
		ActionSummary: fmt.Sprintf("MCP tool call: %s", toolID),
	}
	c.Set(telemetry.CtxActionType, classificationResult.ActionType)

	// Handle approval resubmission if token is present
	if approvalToken != "" {
		return d.handleMCPApprovedCall(c, tenant, agentID, toolID, requestID, requestHash, classificationResult, approvalToken, approvalRequestID, req)
	}

	// Check for risk overrides
	if overrideRisk, lookupErr := d.Store.GetRiskOverride(ctx, tenant.TenantID, classificationResult.ActionType); lookupErr == nil {
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
		"arguments":      params.Arguments,
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
				Data: mustMarshalJSON(map[string]interface{}{
					"reason_code":    invalidReason,
					"policy_version": policyVersion,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("policy evaluation failed")), nil
	}

	// Set default decision if empty
	if decisionResult.Decision == "" {
		decisionResult.Decision = decisionDeny
		decisionResult.Rule.ID = "rule_default_deny"
		decisionResult.Reasons = []models.DecisionReason{{Code: "DEFAULT_DENY", Message: "Default deny"}}
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
	case decisionDeny:
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(decisionDeny, classificationResult.ActionType).Inc()
		}
		if err := d.Store.InsertADR(ctx, adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}

		// Return JSON-RPC error for DENY
		denyData, _ := json.Marshal(map[string]interface{}{
			"denied":         true,
			"policy_version": decisionResult.PolicyVersion,
			"rule_id":        decisionResult.Rule.ID,
			"risk":           decisionResult.Risk,
			"reasons":        formatReasons(decisionResult.Reasons),
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

		if err := d.Store.InsertApprovalRequest(ctx, approval); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist approval")), nil
		}
		if err := d.Store.InsertADR(ctx, adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}

		// Return JSON-RPC error for REQUIRE_APPROVAL
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorApprovalRequired,
			Message: "approval required",
			Data: mustMarshalJSON(map[string]interface{}{
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

	case "ALLOW":
		// TODO: Story 4 - Forward to upstream MCP server
		// For now, return a stub response indicating execution would happen
		if err := d.Store.InsertADR(ctx, adr); err != nil {
			return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to persist decision")), nil
		}

		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues("ALLOW", classificationResult.ActionType).Inc()
		}

		// TODO: This should forward to upstream and return actual result
		// For now, return a placeholder success response
		return mcp.NewSuccessResponse(req.ID, mcp.ToolsCallResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "Tool execution approved. Upstream forwarding not yet implemented (Story 4).",
				},
			},
		})

	default:
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "unknown decision",
			Data: mustMarshalJSON(map[string]interface{}{
				"decision": decisionResult.Decision,
			}),
		}), nil
	}
}

// handleMCPApprovedCall handles MCP tool calls with approval tokens.
func (d *Dependencies) handleMCPApprovedCall(c *echo.Context, tenant models.Tenant, agentID string, toolID string, requestID string, requestHash string, classificationResult classification.Result, approvalToken string, approvalRequestID string, req *mcp.Request) (*mcp.Response, error) {
	ctx := c.Request().Context()

	if approvalRequestID == "" {
		return mcp.NewErrorResponse(req.ID, mcp.NewInvalidParamsError("_rbitr_approval_request_id is required when _rbitr_approval_token is provided")), nil
	}

	approval, err := d.Store.GetApprovalRequest(ctx, tenant.TenantID, approvalRequestID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval not found",
				Data: mustMarshalJSON(map[string]interface{}{
					"reason":              "approval_not_found",
					"approval_request_id": approvalRequestID,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to lookup approval")), nil
	}

	// Verify request hash matches to prevent token reuse across different requests
	if approval.RequestHash != requestHash {
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval request hash mismatch",
			Data: mustMarshalJSON(map[string]interface{}{
				"reason":              "hash_mismatch",
				"approval_request_id": approvalRequestID,
			}),
		}), nil
	}

	// Validate approval state
	now := time.Now().UTC()
	if approval.Status == "EXECUTED" {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
		}
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval already executed",
			Data: mustMarshalJSON(map[string]interface{}{
				"reason":              "already_executed",
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	if approval.Status == "EXPIRED" || now.After(approval.ExpiresAt) {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("expired").Inc()
		}
		_ = d.Store.MarkApprovalExpired(ctx, tenant.TenantID, approval.ApprovalRequestID, now)
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval expired",
			Data: mustMarshalJSON(map[string]interface{}{
				"reason":              "expired",
				"approval_request_id": approval.ApprovalRequestID,
				"expired_at":          approval.ExpiresAt.Format(time.RFC3339),
			}),
		}), nil
	}

	if approval.Status != "APPROVED" {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("not_approved").Inc()
		}
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval not approved",
			Data: mustMarshalJSON(map[string]interface{}{
				"reason":              "not_approved",
				"status":              approval.Status,
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	// Validate approval token
	if utils.HashString(approvalToken) != approval.ApprovalTokenHash {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("token_invalid").Inc()
		}
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorDeniedByPolicy,
			Message: "approval token invalid",
			Data: mustMarshalJSON(map[string]interface{}{
				"reason":              "token_invalid",
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	// Set context for telemetry
	c.Set(telemetry.CtxActionType, approval.ActionType)
	c.Set(telemetry.CtxDecision, "ALLOW")

	// TODO Story 4: Execute actual tool call via upstream MCP server
	// For now, return success response indicating execution would happen
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

	// Mark approval as executed FIRST (before ADR) to prevent race condition
	// If two concurrent requests both pass validation, only one will succeed here
	executedAt := time.Now().UTC()
	if err := d.Store.MarkApprovalExecuted(ctx, tenant.TenantID, approval.ApprovalRequestID, requestID, decisionID, executedAt); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("mark_executed_failed").Inc()
		}
		// Check if this is a concurrent execution (already executed)
		if errors.Is(err, store.ErrInvalidState) {
			return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
				Code:    mcp.ErrorDeniedByPolicy,
				Message: "approval already executed",
				Data: mustMarshalJSON(map[string]interface{}{
					"reason":              "already_executed",
					"approval_request_id": approval.ApprovalRequestID,
				}),
			}), nil
		}
		return mcp.NewErrorResponse(req.ID, mcp.NewInternalError("failed to mark approval executed")), nil
	}

	// Now write ADR after successful state transition
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
		ResponseHash:      "", // Will be filled by upstream execution in Story 4
		ApprovalRequestID: approval.ApprovalRequestID,
		CreatedAt:         executedAt,
	}

	if err := d.Store.InsertADR(ctx, adr); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("adr_failed").Inc()
		}
		c.Logger().Error("failed to persist ADR after approval execution",
			"error", err,
			"approval_request_id", approval.ApprovalRequestID,
			"decision_id", decisionID,
		)
		return mcp.NewErrorResponse(req.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: "failed to persist execution evidence",
			Data: mustMarshalJSON(map[string]interface{}{
				"approval_request_id": approval.ApprovalRequestID,
			}),
		}), nil
	}

	if d.Metrics != nil {
		d.Metrics.ApprovalsExecuteTotal.WithLabelValues("success").Inc()
		d.Metrics.DecisionsTotal.WithLabelValues("ALLOW", actionType).Inc()
		d.Metrics.ToolExecTotal.Inc()
	}

	// Return success response (stub until Story 4 implements upstream forwarding)
	return mcp.NewSuccessResponse(req.ID, mcp.ToolsCallResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: "Tool execution approved and recorded. Upstream forwarding not yet implemented (Story 4).",
			},
		},
	})
}

// formatReasons formats decision reasons for MCP error responses.
func formatReasons(reasons []models.DecisionReason) []string {
	result := make([]string, len(reasons))
	for i, r := range reasons {
		result[i] = r.Message
	}
	return result
}

// mustMarshalJSON marshals a value to JSON, panicking on error.
// Used for known-good data structures in error responses.
func mustMarshalJSON(v interface{}) json.RawMessage {
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
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
