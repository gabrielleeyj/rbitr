package public

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type Dependencies struct {
	Store             store.StoreAPI
	Policy            policy.EvaluatorAPI
	Connector         connector.Connector
	Metrics           *telemetry.Metrics
	Config            config.Config
	Notifier          *notifications.Service
	ToolCache         *cache.TTLCache[models.Tool]
	RiskOverrideCache *cache.TTLCache[string]
}

type ToolCallRequest struct {
	HTTPMethod string            `json:"http_method"`
	Path       string            `json:"path"`
	Query      string            `json:"query"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

type ToolCallResponse struct {
	Error             string            `json:"error,omitempty"`
	RequestID         string            `json:"request_id"`
	Decision          string            `json:"decision"`
	Reason            string            `json:"reason"`
	ApprovalRequestID string            `json:"approval_request_id,omitempty"`
	ApprovalToken     string            `json:"approval_token,omitempty"`
	ExpiresAt         string            `json:"expires_at,omitempty"`
	ActionType        string            `json:"action_type,omitempty"`
	Risk              string            `json:"risk,omitempty"`
	ToolStatus        int               `json:"tool_status,omitempty"`
	ToolHeaders       map[string]string `json:"tool_headers,omitempty"`
	ToolBody          string            `json:"tool_body,omitempty"`
	ShadowDenied      bool              `json:"shadow_denied,omitempty"`
	RbitrShadow       map[string]any    `json:"_rbitr_shadow,omitempty"`
}

type EvidenceResponse struct {
	TenantID string                        `json:"tenant_id"`
	Records  []models.ActionDecisionExport `json:"records"`
}

const decisionDeny = "DENY"
const decisionInvalidReason = "policy output invalid"
const approvalHeaderID = "X-Approval-Request-Id"
const approvalHeaderToken = "X-Approval-Token"
const defaultApprovalTTL = 15 * time.Minute
const approvalExecutionRetryWindow = 60 * time.Second
const xTenantKeySunset = "Tue, 31 Mar 2026 00:00:00 GMT"

var errToolNotFound = errors.New("tool not found")
var errConnectorMissing = errors.New("connector not configured")

func RegisterRoutes(e *echo.Echo, deps *Dependencies) {
	v1 := e.Group("/v1")
	v1.POST("/tools/:tool_id/call", deps.handleToolCall)
	v1.GET("/tenants/:tenant_id/evidence", deps.handleEvidence)
	v1.GET("/mcp/:tenant_id", deps.handleMCPStream)
	v1.POST("/mcp/:tenant_id", deps.handleMCP)
}

func (d *Dependencies) handleToolCall(c *echo.Context) error {
	start := time.Now()
	if d.Metrics != nil {
		d.Metrics.GatewayRequests.Inc()
	}

	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}

	tenant, agentID, err := d.authenticateTenantRequest(c)
	if err != nil {
		return authError(c, err)
	}

	toolID := c.Param("tool_id")
	c.Set(telemetry.CtxTenantID, tenant.TenantID)
	c.Set(telemetry.CtxAgentID, agentID)
	c.Set(telemetry.CtxToolID, toolID)
	requestID := c.Request().Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	c.Set(telemetry.CtxRequestID, requestID)
	c.Response().Header().Set("X-Request-Id", requestID)
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")

	var payload ToolCallRequest
	if decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload); decodeErr != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	payload.HTTPMethod = strings.ToUpper(strings.TrimSpace(payload.HTTPMethod))
	if payload.HTTPMethod == "" || payload.Path == "" {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "http_method and path required"})
	}

	bodyBytes := []byte(payload.Body)
	if int64(len(bodyBytes)) > d.Config.BodyLimitSize {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error": "body too large"})
	}

	filteredHeaders := utils.FilterHeaders(payload.Headers)
	bodyHash := utils.HashBody(bodyBytes)
	canonical := utils.CanonicalRequest{
		TenantID:       tenant.TenantID,
		AgentID:        agentID,
		ToolID:         toolID,
		Method:         payload.HTTPMethod,
		Path:           payload.Path,
		Query:          payload.Query,
		Headers:        filteredHeaders,
		BodyHash:       bodyHash,
		IdempotencyKey: idempotencyKey,
	}
	requestHash := utils.HashCanonical(&canonical)

	classificationResult := classification.Classify(toolID, payload.HTTPMethod, payload.Path, payload.Query, filteredHeaders)
	c.Set(telemetry.CtxActionType, classificationResult.ActionType)
	approvalID := c.Request().Header.Get(approvalHeaderID)
	approvalToken := c.Request().Header.Get(approvalHeaderToken)
	if approvalID != "" || approvalToken != "" {
		if approvalID == "" || approvalToken == "" {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "approval headers required"})
		}
		return d.handleApprovedToolCall(c, approvedToolCallParams{
			tenantID:              tenant.TenantID,
			agentID:               agentID,
			toolID:                toolID,
			requestID:             requestID,
			requestHash:           requestHash,
			classificationRisk:    classificationResult.ActionRisk,
			classificationType:    classificationResult.ActionType,
			classificationSummary: classificationResult.ActionSummary,
			payload:               payload,
			bodyBytes:             bodyBytes,
			filteredHeaders:       filteredHeaders,
			approvalID:            approvalID,
			approvalToken:         approvalToken,
		})
	}
	if overrideRisk, lookupErr := d.getRiskOverrideCached(c.Request().Context(), tenant.TenantID, classificationResult.ActionType); lookupErr == nil {
		classificationResult.ActionRisk = overrideRisk
	} else if !errors.Is(lookupErr, store.ErrNotFound) {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "risk override lookup failed"})
	}

	if d.Policy == nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "policy evaluator not configured"})
	}
	policyInput := map[string]any{
		"tenant_id":      tenant.TenantID,
		"agent_id":       agentID,
		"tool_id":        toolID,
		"action_type":    classificationResult.ActionType,
		"action_risk":    classificationResult.ActionRisk,
		"policy_version": "",
		"request": map[string]any{
			"method": payload.HTTPMethod,
			"path":   payload.Path,
		},
	}
	decisionResult, err := d.Policy.Evaluate(c.Request().Context(), tenant.TenantID, policyInput)
	if err != nil {
		if invalidReason, policyVersion, ok := policyInvalidReason(err); ok {
			d.emitNotification(c, tenant.TenantID, notifications.EventPolicyInvalidOutput, notifications.SeverityCritical, policyVersion, map[string]string{
				"Tenant":        tenant.TenantID,
				"Tool":          toolID,
				"Action":        classificationResult.ActionType,
				"PolicyVersion": policyVersion,
				"reason":        invalidReason,
			})
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
				d.Metrics.PolicyEvalInvalidTotal.WithLabelValues(invalidReason).Inc()
			}
			c.Logger().Error("policy output invalid",
				"error", err,
				"tenant_id", tenant.TenantID,
				"agent_id", agentID,
				"tool_id", toolID,
				"policy_version", policyVersion,
				"request_id", requestID,
			)
			decisionResult = policy.Result{
				Version:       "invalid",
				Decision:      decisionDeny,
				Risk:          classificationResult.ActionRisk,
				Rule:          models.DecisionRule{ID: "policy_invalid", Priority: 1000},
				Reasons:       []models.DecisionReason{{Code: "POLICY_OUTPUT_INVALID", Message: decisionInvalidReason}},
				Constraints:   map[string]any{},
				PolicyVersion: policyVersion,
			}
		} else {
			d.emitNotification(c, tenant.TenantID, notifications.EventPolicyEvalError, notifications.SeverityCritical, "", map[string]string{
				"Tenant": tenant.TenantID,
				"Tool":   toolID,
				"Action": classificationResult.ActionType,
				"reason": err.Error(),
			})
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "policy evaluation failed"})
		}
	}
	if d.Metrics != nil {
		d.Metrics.DecisionLatencyMs.Observe(float64(time.Since(start).Milliseconds()))
	}
	if decisionResult.Decision == "" {
		decisionResult.Decision = decisionDeny
		decisionResult.Rule.ID = "rule_default_deny"
		decisionResult.Reasons = []models.DecisionReason{{Code: "DEFAULT_DENY", Message: "Default deny"}}
	}
	decisionResult.Constraints = withMatchedRulesConstraint(decisionResult.Constraints, decisionResult.MatchedRules)
	enforcementMode, enforcementErr := d.tenantEnforcementMode(c.Request().Context(), tenant.TenantID)
	if enforcementErr != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant enforcement mode"})
	}

	rateLimitViolation, err := d.enforceRateLimit(
		c.Request().Context(),
		tenant.TenantID,
		agentID,
		toolID,
		classificationResult.ActionType,
		decisionResult.Constraints,
	)
	if err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "rate limit check failed"})
	}
	if rateLimitViolation != nil {
		c.Set(telemetry.CtxDecision, decisionDeny)

		decisionID := "d_" + uuid.NewString()
		reasons := []models.DecisionReason{{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "Rate limit exceeded",
		}}
		rateLimitConstraints := withRateLimitConstraint(decisionResult.Constraints, rateLimitViolation)
		adr := models.ActionDecisionRecord{
			DecisionID:      decisionID,
			RequestID:       requestID,
			TenantID:        tenant.TenantID,
			AgentID:         agentID,
			ToolID:          toolID,
			ActionType:      classificationResult.ActionType,
			ActionRisk:      classificationResult.ActionRisk,
			ActionSummary:   classificationResult.ActionSummary,
			Decision:        decisionDeny,
			DecisionVersion: decisionResult.Version,
			DecisionRisk:    decisionResult.Risk,
			RuleID:          "rate_limit_" + rateLimitViolation.Window,
			RulePriority:    1000,
			Reasons:         reasons,
			Constraints:     rateLimitConstraints,
			Tags:            decisionResult.Tags,
			PolicyVersion:   decisionResult.PolicyVersion,
			Reason:          firstReasonMessage(reasons),
			RequestHash:     requestHash,
			CreatedAt:       time.Now().UTC(),
		}
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(decisionDeny, classificationResult.ActionType).Inc()
		}
		if isShadowMode(enforcementMode) {
			return d.executeRESTShadowDeny(
				c,
				tenant.TenantID,
				toolID,
				requestID,
				payload,
				bodyBytes,
				filteredHeaders,
				reasons,
				buildShadowDecisionMetadata("rate_limit_"+rateLimitViolation.Window, decisionResult.Risk, decisionResult.PolicyVersion, reasons, rateLimitConstraints),
			)
		}
		return c.JSON(http.StatusTooManyRequests, map[string]any{
			"code":                "RATE_LIMIT_EXCEEDED",
			"window":              rateLimitViolation.Window,
			"limit":               rateLimitViolation.Limit,
			"retry_after_seconds": rateLimitViolation.RetryAfterSeconds,
			"scope":               rateLimitViolation.Scope,
		})
	}

	argConstraintViolation := d.enforceArgumentConstraints(decisionResult.Constraints, parseRESTArguments(bodyBytes))
	if argConstraintViolation != nil {
		c.Set(telemetry.CtxDecision, decisionDeny)

		decisionID := "d_" + uuid.NewString()
		reasons := []models.DecisionReason{{
			Code:    argConstraintViolation.ReasonCode,
			Message: argConstraintViolation.Message,
		}}
		argConstraints := withArgConstraintFailures(decisionResult.Constraints, argConstraintViolation)
		adr := models.ActionDecisionRecord{
			DecisionID:      decisionID,
			RequestID:       requestID,
			TenantID:        tenant.TenantID,
			AgentID:         agentID,
			ToolID:          toolID,
			ActionType:      classificationResult.ActionType,
			ActionRisk:      classificationResult.ActionRisk,
			ActionSummary:   classificationResult.ActionSummary,
			Decision:        decisionDeny,
			DecisionVersion: decisionResult.Version,
			DecisionRisk:    decisionResult.Risk,
			RuleID:          argConstraintRuleID(argConstraintViolation),
			RulePriority:    1000,
			Reasons:         reasons,
			Constraints:     argConstraints,
			Tags:            decisionResult.Tags,
			PolicyVersion:   decisionResult.PolicyVersion,
			Reason:          firstReasonMessage(reasons),
			RequestHash:     requestHash,
			CreatedAt:       time.Now().UTC(),
		}
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(decisionDeny, classificationResult.ActionType).Inc()
		}
		if isShadowMode(enforcementMode) {
			return d.executeRESTShadowDeny(
				c,
				tenant.TenantID,
				toolID,
				requestID,
				payload,
				bodyBytes,
				filteredHeaders,
				reasons,
				buildShadowDecisionMetadata(argConstraintRuleID(argConstraintViolation), decisionResult.Risk, decisionResult.PolicyVersion, reasons, argConstraints),
			)
		}
		return c.JSON(http.StatusForbidden, ToolCallResponse{
			RequestID: requestID,
			Decision:  decisionDeny,
			Reason:    firstReasonMessage(reasons),
		})
	}
	c.Set(telemetry.CtxDecision, decisionResult.Decision)

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

	switch decisionResult.Decision {
	case decisionDeny:
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues(decisionDeny, classificationResult.ActionType).Inc()
		}
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		if isShadowMode(enforcementMode) {
			return d.executeRESTShadowDeny(
				c,
				tenant.TenantID,
				toolID,
				requestID,
				payload,
				bodyBytes,
				filteredHeaders,
				decisionResult.Reasons,
				buildShadowDecisionMetadata(decisionResult.Rule.ID, decisionResult.Risk, decisionResult.PolicyVersion, decisionResult.Reasons, decisionResult.Constraints),
			)
		}
		return c.JSON(http.StatusForbidden, ToolCallResponse{
			RequestID: requestID,
			Decision:  decisionDeny,
			Reason:    firstReasonMessage(decisionResult.Reasons),
		})
	case "REQUIRE_APPROVAL":
		now := time.Now().UTC()
		token, err := generateApprovalToken()
		if err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate approval token"})
		}
		actionRisk := decisionResult.Risk
		if actionRisk == "" {
			actionRisk = classificationResult.ActionRisk
		}
		expiresAt := now.Add(approvalTTL(c.Request().Context(), d.Store, decisionResult.Constraints))
		approvalID := "ar_" + uuid.NewString()
		approval := models.ApprovalRequest{
			ApprovalRequestID: approvalID,
			TenantID:          tenant.TenantID,
			AgentID:           agentID,
			ToolID:            toolID,
			ActionType:        classificationResult.ActionType,
			RequestHash:       requestHash,
			RequestContext:    buildApprovalRequestContext(payload),
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
		if err := d.Store.InsertApprovalRequest(c.Request().Context(), approval); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist approval"})
		}
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		return c.JSON(http.StatusConflict, ToolCallResponse{
			Error:             "approval_required",
			RequestID:         requestID,
			Decision:          "REQUIRE_APPROVAL",
			Reason:            firstReasonMessage(decisionResult.Reasons),
			ApprovalRequestID: approvalID,
			ApprovalToken:     token,
			ExpiresAt:         expiresAt.Format(time.RFC3339),
			ActionType:        classificationResult.ActionType,
			Risk:              actionRisk,
		})
	case "ALLOW":
		toolStart := time.Now()
		resp, err := d.executeToolCall(c.Request().Context(), tenant.TenantID, toolID, payload, bodyBytes, filteredHeaders)
		if err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			if errors.Is(err, errToolNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
			}
			if errors.Is(err, errConnectorMissing) {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "connector not configured"})
			}
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "tool execution failed"})
		}
		if d.Metrics != nil {
			d.Metrics.ToolLatencyMs.Observe(float64(time.Since(toolStart).Milliseconds()))
		}
		adr.ResponseHash = resp.BodyHash
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			if d.Metrics != nil {
				d.Metrics.ErrorsTotal.Inc()
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}

		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues("ALLOW", classificationResult.ActionType).Inc()
			d.Metrics.ToolExecTotal.Inc()
		}

		return c.JSON(http.StatusOK, ToolCallResponse{
			RequestID:   requestID,
			Decision:    "ALLOW",
			Reason:      firstReasonMessage(decisionResult.Reasons),
			ToolStatus:  resp.Status,
			ToolHeaders: resp.Headers,
			ToolBody:    string(resp.Body),
		})
	default:
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "unknown decision"})
	}
}

type approvedToolCallParams struct {
	tenantID              string
	agentID               string
	toolID                string
	requestID             string
	requestHash           string
	classificationRisk    string
	classificationType    string
	classificationSummary string
	payload               ToolCallRequest
	bodyBytes             []byte
	filteredHeaders       map[string]string
	approvalID            string
	approvalToken         string
}

func (d *Dependencies) handleApprovedToolCall(c *echo.Context, params approvedToolCallParams) error {
	ctx := c.Request().Context()
	approval, err := d.Store.GetApprovalForExecution(ctx, params.tenantID, params.approvalID)
	if err != nil {
		d.Metrics.ErrorsTotal.Inc()
		if errors.Is(err, store.ErrNotFound) {
			d.emitTokenAbuse(c, params, "approval_not_found")
			return approvalError(c, http.StatusForbidden, "approval_token_invalid")
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "approval lookup failed"})
	}

	now := time.Now().UTC()
	tokenHash := utils.HashString(params.approvalToken)
	if !utils.SecureCompare(tokenHash, approval.ApprovalTokenHash) {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("token_invalid").Inc()
		}
		d.emitTokenAbuse(c, params, "approval_token_invalid")
		return approvalError(c, http.StatusForbidden, "approval_token_invalid")
	}
	if approval.RequestHash != params.requestHash {
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("hash_mismatch").Inc()
		}
		d.emitTokenAbuse(c, params, "approval_request_hash_mismatch")
		return approvalError(c, http.StatusForbidden, "approval_request_hash_mismatch")
	}
	if approval.Status == "EXPIRED" || now.After(approval.ExpiresAt) {
		_ = d.Store.MarkApprovalExpired(ctx, params.tenantID, params.approvalID, now)
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("expired").Inc()
		}
		return approvalError(c, http.StatusForbidden, "approval_expired")
	}

	switch approval.Status {
	case "APPROVED":
		if err := d.Store.ClaimApprovalExecution(ctx, params.tenantID, params.approvalID, tokenHash, params.requestHash, now); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return approvalError(c, http.StatusForbidden, "approval_token_invalid")
			}
			if errors.Is(err, store.ErrInvalidState) {
				latest, stateErr := d.Store.GetApprovalForExecution(ctx, params.tenantID, params.approvalID)
				if stateErr != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "approval lookup failed"})
				}

				switch latest.Status {
				case "EXECUTING":
					if !approvalRetryAllowed(latest, now) {
						if d.Metrics != nil {
							d.Metrics.ApprovalsExecuteTotal.WithLabelValues("retry_window_exceeded").Inc()
						}
						return approvalError(c, http.StatusConflict, "approval_retry_window_exceeded")
					}
				case "EXECUTED":
					if d.Metrics != nil {
						d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
					}
					return approvalError(c, http.StatusConflict, "approval_already_executed")

				default:
					if d.Metrics != nil {
						d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_claimed").Inc()
					}
					return approvalError(c, http.StatusConflict, "approval_already_claimed")
				}
				approval = latest
			} else {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to claim approval"})
			}
		} else {
			approval.Status = "EXECUTING"
			approval.ExecutingAt = &now
			if approval.ExecutionID == "" {
				approval.ExecutionID = approval.ApprovalRequestID
			}
		}
	case "EXECUTING":
		if !approvalRetryAllowed(approval, now) {
			if d.Metrics != nil {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("retry_window_exceeded").Inc()
			}
			return approvalError(c, http.StatusConflict, "approval_retry_window_exceeded")
		}
	case "EXECUTED":
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_executed").Inc()
		}
		return approvalError(c, http.StatusConflict, "approval_already_executed")
	default:
		if d.Metrics != nil {
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("already_claimed").Inc()
		}
		return approvalError(c, http.StatusConflict, "approval_already_claimed")
	}

	actionType := approval.ActionType
	if actionType == "" {
		actionType = params.classificationType
	}
	actionRisk := approval.Risk
	if actionRisk == "" {
		actionRisk = params.classificationRisk
	}
	actionSummary := approval.ActionSummary
	if actionSummary == "" {
		actionSummary = params.classificationSummary
	}
	c.Set(telemetry.CtxActionType, actionType)
	c.Set(telemetry.CtxDecision, "ALLOW")

	toolStart := time.Now()
	resp, err := d.executeToolCall(ctx, params.tenantID, params.toolID, params.payload, params.bodyBytes, params.filteredHeaders)
	if err != nil {
		errorCode := "UPSTREAM_ERROR"
		httpStatus := http.StatusBadGateway
		responseCode := "upstream_error"
		if isUpstreamTimeoutError(err) {
			errorCode = "UPSTREAM_TIMEOUT"
			httpStatus = http.StatusGatewayTimeout
			responseCode = "upstream_timeout"
		}
		_ = d.Store.MarkApprovalExecutionFailed(ctx, params.tenantID, params.approvalID, errorCode, time.Now().UTC())
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			if errorCode == "UPSTREAM_TIMEOUT" {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("upstream_timeout").Inc()
			} else {
				d.Metrics.ApprovalsExecuteTotal.WithLabelValues("upstream_failed").Inc()
			}
		}
		return c.JSON(httpStatus, map[string]string{"error": responseCode})
	}
	if d.Metrics != nil {
		d.Metrics.ToolLatencyMs.Observe(float64(time.Since(toolStart).Milliseconds()))
	}

	decisionID := "d_" + uuid.NewString()
	ruleID := approval.RuleID
	if ruleID == "" {
		ruleID = "approval_granted"
	}
	policyVersion := approval.PolicyVersion
	if policyVersion == "" {
		policyVersion = "unknown"
	}
	reasons := []models.DecisionReason{{Code: "APPROVED", Message: "Approved execution"}}
	adr := models.ActionDecisionRecord{
		DecisionID:        decisionID,
		RequestID:         params.requestID,
		TenantID:          params.tenantID,
		AgentID:           params.agentID,
		ToolID:            params.toolID,
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
		RequestHash:       params.requestHash,
		ResponseHash:      resp.BodyHash,
		ApprovalRequestID: approval.ApprovalRequestID,
		CreatedAt:         time.Now().UTC(),
	}
	if err := d.Store.MarkApprovalExecuted(ctx, params.tenantID, params.approvalID, params.requestID, decisionID, time.Now().UTC()); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("update_failed").Inc()
		}
		if errors.Is(err, store.ErrInvalidState) {
			return approvalError(c, http.StatusConflict, "approval_already_claimed")
		}
		if errors.Is(err, store.ErrNotFound) {
			return approvalError(c, http.StatusForbidden, "approval_token_invalid")
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}
	if err := d.Store.InsertADR(ctx, adr); err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("adr_failed").Inc()
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
	}

	if d.Metrics != nil {
		if d.Metrics != nil {
			d.Metrics.DecisionsTotal.WithLabelValues("ALLOW", actionType).Inc()
			d.Metrics.ToolExecTotal.Inc()
			d.Metrics.ApprovalsExecuteTotal.WithLabelValues("success").Inc()
		}
	}

	return c.JSON(http.StatusOK, ToolCallResponse{
		RequestID:   params.requestID,
		Decision:    "ALLOW",
		Reason:      firstReasonMessage(reasons),
		ToolStatus:  resp.Status,
		ToolHeaders: resp.Headers,
		ToolBody:    string(resp.Body),
	})
}

func (d *Dependencies) getToolCached(ctx context.Context, tenantID, toolID string) (models.Tool, error) {
	cacheKey := tenantID + ":" + toolID
	if d.ToolCache != nil {
		if tool, ok := d.ToolCache.Get(cacheKey); ok {
			d.recordCacheMetric("tool", true)
			return tool, nil
		}
		d.recordCacheMetric("tool", false)
	}
	tool, err := d.Store.GetTool(ctx, tenantID, toolID)
	if err != nil {
		return models.Tool{}, err
	}
	if d.ToolCache != nil {
		d.ToolCache.Set(cacheKey, tool)
	}
	return tool, nil
}

func (d *Dependencies) getRiskOverrideCached(ctx context.Context, tenantID, actionType string) (string, error) {
	cacheKey := tenantID + ":" + actionType
	if d.RiskOverrideCache != nil {
		if risk, ok := d.RiskOverrideCache.Get(cacheKey); ok {
			d.recordCacheMetric("risk_override", true)
			return risk, nil
		}
		d.recordCacheMetric("risk_override", false)
	}
	risk, err := d.Store.GetRiskOverride(ctx, tenantID, actionType)
	if err != nil {
		return "", err
	}
	if d.RiskOverrideCache != nil {
		d.RiskOverrideCache.Set(cacheKey, risk)
	}
	return risk, nil
}

func (d *Dependencies) recordCacheMetric(cacheName string, hit bool) {
	if d.Metrics == nil {
		return
	}
	if hit {
		d.Metrics.CacheHitsTotal.WithLabelValues(cacheName).Inc()
	} else {
		d.Metrics.CacheMissesTotal.WithLabelValues(cacheName).Inc()
	}
}

func (d *Dependencies) executeToolCall(ctx context.Context, tenantID, toolID string, payload ToolCallRequest, bodyBytes []byte, filteredHeaders map[string]string) (connector.Response, error) {
	tool, err := d.getToolCached(ctx, tenantID, toolID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return connector.Response{}, errToolNotFound
		}
		return connector.Response{}, err
	}
	if d.Connector == nil {
		return connector.Response{}, errConnectorMissing
	}
	url := strings.TrimRight(tool.BaseURL, "/") + payload.Path
	if payload.Query != "" {
		if strings.HasPrefix(payload.Query, "?") {
			url += payload.Query
		} else {
			url += "?" + payload.Query
		}
	}

	forwardHeaders := make(map[string]string)
	maps.Copy(forwardHeaders, filteredHeaders)
	applyToolAuth(forwardHeaders, &tool)

	return d.Connector.Execute(ctx, connector.Request{
		Method:  payload.HTTPMethod,
		URL:     url,
		Headers: forwardHeaders,
		Body:    bodyBytes,
	})
}

func approvalTTL(ctx context.Context, st store.StoreAPI, constraints map[string]any) time.Duration {
	if constraints == nil {
		return defaultApprovalTTLFromStore(ctx, st)
	}
	rawApproval, ok := constraints["approval"]
	if !ok {
		return defaultApprovalTTLFromStore(ctx, st)
	}
	approval, ok := rawApproval.(map[string]any)
	if !ok {
		return defaultApprovalTTLFromStore(ctx, st)
	}
	rawTTL, ok := approval["expires_in_seconds"]
	if !ok {
		return defaultApprovalTTLFromStore(ctx, st)
	}
	switch value := rawTTL.(type) {
	case float64:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	case int:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	case int64:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Second
		}
	}
	return defaultApprovalTTLFromStore(ctx, st)
}

func defaultApprovalTTLFromStore(ctx context.Context, st store.StoreAPI) time.Duration {
	if st == nil {
		return defaultApprovalTTL
	}
	seconds, err := st.GetDefaultApprovalTTLSeconds(ctx)
	if err != nil || seconds <= 0 {
		return defaultApprovalTTL
	}
	return time.Duration(seconds) * time.Second
}

func generateApprovalToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func approvalError(c *echo.Context, status int, code string) error {
	return c.JSON(status, map[string]string{"error": code})
}

func (d *Dependencies) emitTokenAbuse(c *echo.Context, params approvedToolCallParams, reason string) {
	d.emitNotification(c, params.tenantID, notifications.EventTokenAbuse, notifications.SeverityCritical, params.approvalID, map[string]string{
		"Tenant":   params.tenantID,
		"Tool":     params.toolID,
		"Action":   params.classificationType,
		"Approval": params.approvalID,
		"reason":   reason,
	})
}

func (d *Dependencies) handleEvidence(c *echo.Context) error {
	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := parsePositiveInt(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	tenant, agentID, err := d.authenticateTenantRequest(c)
	if err != nil {
		return authError(c, err)
	}
	c.Set(telemetry.CtxTenantID, tenant.TenantID)
	c.Set(telemetry.CtxAgentID, agentID)

	requestedTenant := c.Param("tenant_id")
	if tenant.TenantID != requestedTenant {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "tenant mismatch"})
	}

	records, err := d.Store.ListEvidence(c.Request().Context(), tenant.TenantID, limit)
	if err != nil {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load evidence"})
	}

	exported := make([]models.ActionDecisionExport, 0, len(records))
	for i := range records {
		record := &records[i]
		export := models.ActionDecisionExport{
			DecisionID:        record.DecisionID,
			RequestID:         record.RequestID,
			TenantID:          record.TenantID,
			AgentID:           record.AgentID,
			ToolID:            record.ToolID,
			ActionType:        record.ActionType,
			ActionRisk:        record.ActionRisk,
			ActionSummary:     record.ActionSummary,
			Decision:          record.Decision,
			DecisionVersion:   record.DecisionVersion,
			DecisionRisk:      record.DecisionRisk,
			RuleID:            record.RuleID,
			RulePriority:      record.RulePriority,
			Reasons:           record.Reasons,
			Constraints:       record.Constraints,
			Tags:              record.Tags,
			PolicyVersion:     record.PolicyVersion,
			Reason:            record.Reason,
			RequestHash:       record.RequestHash,
			ResponseHash:      record.ResponseHash,
			ApprovalRequestID: record.ApprovalRequestID,
			Timestamp:         record.CreatedAt,
		}
		if record.ApprovalRequestID != "" {
			if approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenant.TenantID, record.ApprovalRequestID); err == nil {
				export.ApprovalStatus = approval.Status
				export.ApprovalDecidedAt = approval.DecidedAt
				export.ApprovalDecidedBy = approval.DecidedBy
				export.ApprovalComment = approval.DecisionComment
				export.ApprovalExecutedAt = approval.ExecutedAt
				export.ApprovalExecutedRequestID = approval.ExecutedRequestID
				export.ApprovalExecutedDecisionID = approval.ExecutedDecisionID
				export.ApprovalRequestDecisionID = approval.RequestDecisionID
			}
		}
		exported = append(exported, export)
	}

	return c.JSON(http.StatusOK, EvidenceResponse{
		TenantID: tenant.TenantID,
		Records:  exported,
	})
}

func (d *Dependencies) emitNotification(c *echo.Context, tenantID, eventType, severity, resourceID string, data map[string]string) {
	if d.Notifier == nil {
		return
	}
	msg := notifications.BuildMessage(eventType, data)
	_ = d.Notifier.Send(c.Request().Context(), tenantID, notifications.NotificationEvent{
		TenantID:   tenantID,
		EventType:  eventType,
		Severity:   severity,
		ResourceID: resourceID,
	}, msg)
}

func (d *Dependencies) authenticateTenantRequest(c *echo.Context) (models.Tenant, string, error) {
	agentID := c.Request().Header.Get(auth.AgentIDHeader)
	tenantKey, usedFallback := auth.TenantKeyFromRequest(c.Request(), d.Config.DisableXTenantKey)
	authResult, err := auth.AuthenticateTenantDetailed(c.Request().Context(), d.Store, tenantKey, agentID)
	if err != nil {
		return models.Tenant{}, "", err
	}
	tenant := authResult.Tenant
	if usedFallback {
		c.Response().Header().Set("Deprecation", "true")
		c.Response().Header().Set("Sunset", xTenantKeySunset)
		if d.Metrics != nil && d.Metrics.TenantAuthFallbackTotal != nil {
			d.Metrics.TenantAuthFallbackTotal.Inc()
		}
		c.Logger().Warn("deprecated tenant auth header used",
			"tenant_id", tenant.TenantID,
			"agent_id", agentID,
			"request_id", c.Request().Header.Get("X-Request-Id"),
		)
	}
	if authResult.LegacyHashUpgraded {
		if d.Metrics != nil && d.Metrics.TenantKeyLegacyUpgradeTotal != nil {
			d.Metrics.TenantKeyLegacyUpgradeTotal.Inc()
		}
		c.Logger().Info("tenant key hash upgraded from legacy sha256",
			"tenant_id", tenant.TenantID,
			"agent_id", agentID,
			"request_id", c.Request().Header.Get("X-Request-Id"),
		)
	}
	return tenant, agentID, nil
}

func authError(c *echo.Context, err error) error {
	if errors.Is(err, auth.ErrUnauthorized) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	if errors.Is(err, auth.ErrInvalidAgentID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent_id: must be 1-128 chars, alphanumeric/underscore/dash/dot only"})
	}
	if errors.Is(err, auth.ErrForbidden) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "auth error"})
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func approvalRetryAllowed(approval models.ApprovalRequest, now time.Time) bool {
	if approval.ExecutingAt == nil {
		return false
	}
	return now.Sub(*approval.ExecutingAt) <= approvalExecutionRetryWindow
}

func buildApprovalRequestContext(payload ToolCallRequest) map[string]any {
	context := map[string]any{
		"http_method": payload.HTTPMethod,
		"path":        payload.Path,
		"query":       payload.Query,
	}
	if len(payload.Headers) > 0 {
		context["headers"] = redactApprovalContextHeaders(payload.Headers)
	}
	body := strings.TrimSpace(payload.Body)
	if body != "" {
		var bodyJSON any
		if err := json.Unmarshal([]byte(body), &bodyJSON); err == nil {
			context["body_json"] = bodyJSON
		} else {
			context["body"] = payload.Body
		}
	}
	return context
}

func redactApprovalContextHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(headers))
	for key, value := range headers {
		if isSensitiveApprovalContextHeader(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func isSensitiveApprovalContextHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "proxy-authorization", "x-api-key", "api-key", "x-admin-key", "x-tenant-key", "cookie", "set-cookie":
		return true
	default:
		return false
	}
}

func isUpstreamTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func applyToolAuth(headers map[string]string, tool *models.Tool) {
	if tool.AuthType == "bearer" && tool.AuthValue != "" {
		headers["Authorization"] = "Bearer " + tool.AuthValue
	}
	if tool.AuthType == "api_key" && tool.AuthValue != "" {
		headers["X-Api-Key"] = tool.AuthValue
	}
}

func firstReasonMessage(reasons []models.DecisionReason) string {
	if len(reasons) == 0 {
		return ""
	}
	return reasons[0].Message
}

func policyInvalidReason(err error) (string, string, bool) {
	var outputErr policy.InvalidPolicyOutputError
	if errors.As(err, &outputErr) {
		return outputErr.Reason, outputErr.PolicyVersion, true
	}
	if errors.Is(err, opa.ErrInvalidPolicyOutput) {
		return "schema_violation", "", true
	}
	return "", "", false
}
