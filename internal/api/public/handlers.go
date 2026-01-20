package public

import (
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/classification"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type Dependencies struct {
	Store     store.StoreAPI
	Policy    policy.EvaluatorAPI
	Connector connector.Connector
	Metrics   *telemetry.Metrics
	Config    config.Config
}

type ToolCallRequest struct {
	HTTPMethod string            `json:"http_method"`
	Path       string            `json:"path"`
	Query      string            `json:"query"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

type ToolCallResponse struct {
	RequestID         string            `json:"request_id"`
	Decision          string            `json:"decision"`
	Reason            string            `json:"reason"`
	ApprovalRequestID string            `json:"approval_request_id,omitempty"`
	ToolStatus        int               `json:"tool_status,omitempty"`
	ToolHeaders       map[string]string `json:"tool_headers,omitempty"`
	ToolBody          string            `json:"tool_body,omitempty"`
}

type EvidenceResponse struct {
	TenantID string                        `json:"tenant_id"`
	Records  []models.ActionDecisionExport `json:"records"`
}

func RegisterRoutes(e *echo.Echo, deps Dependencies) {
	v1 := e.Group("/v1")
	v1.POST("/tools/:tool_id/call", deps.handleToolCall)
	v1.GET("/tenants/:tenant_id/evidence", deps.handleEvidence)
}

func (d Dependencies) handleToolCall(c *echo.Context) error {
	start := time.Now()
	d.Metrics.GatewayRequests.Inc()

	tenantKey := c.Request().Header.Get(auth.TenantKeyHeader)
	agentID := c.Request().Header.Get(auth.AgentIDHeader)
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}

	tenant, err := auth.AuthenticateTenant(c.Request().Context(), d.Store, tenantKey, agentID)
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
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	payload.HTTPMethod = strings.ToUpper(strings.TrimSpace(payload.HTTPMethod))
	if payload.HTTPMethod == "" || payload.Path == "" {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "http_method and path required"})
	}

	bodyBytes := []byte(payload.Body)
	if int64(len(bodyBytes)) > d.Config.BodyLimitSize {
		d.Metrics.ErrorsTotal.Inc()
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
	requestHash := utils.HashCanonical(canonical)

	classificationResult := classification.Classify(toolID, payload.HTTPMethod, payload.Path, payload.Query, filteredHeaders)
	c.Set(telemetry.CtxActionType, classificationResult.ActionType)
	if overrideRisk, err := d.Store.GetRiskOverride(c.Request().Context(), tenant.TenantID, classificationResult.ActionType); err == nil {
		classificationResult.ActionRisk = overrideRisk
	} else if err != nil && err != store.ErrNotFound {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "risk override lookup failed"})
	}

	if d.Policy == nil {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "policy evaluator not configured"})
	}
	policyInput := map[string]any{
		"tenant_id":   tenant.TenantID,
		"tool_id":     toolID,
		"action_type": classificationResult.ActionType,
		"action_risk": classificationResult.ActionRisk,
		"method":      payload.HTTPMethod,
		"path":        payload.Path,
	}
	decisionResult, err := d.Policy.Evaluate(c.Request().Context(), tenant.TenantID, policyInput)
	if err != nil {
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "policy evaluation failed"})
	}
	d.Metrics.DecisionLatencyMs.Observe(float64(time.Since(start).Milliseconds()))
	if decisionResult.Decision == "" {
		decisionResult.Decision = "DENY"
		decisionResult.RuleID = "rule_default_deny"
		decisionResult.Reason = "Default deny"
	}
	c.Set(telemetry.CtxDecision, decisionResult.Decision)

	decisionID := "d_" + uuid.NewString()
	adr := models.ActionDecisionRecord{
		DecisionID:    decisionID,
		RequestID:     requestID,
		TenantID:      tenant.TenantID,
		AgentID:       agentID,
		ToolID:        toolID,
		ActionType:    classificationResult.ActionType,
		ActionRisk:    classificationResult.ActionRisk,
		ActionSummary: classificationResult.ActionSummary,
		Decision:      decisionResult.Decision,
		Reason:        decisionResult.Reason,
		RuleID:        decisionResult.RuleID,
		PolicyVersion: decisionResult.PolicyVersion,
		RequestHash:   requestHash,
		CreatedAt:     time.Now().UTC(),
	}

	switch decisionResult.Decision {
	case "DENY":
		d.Metrics.DecisionsTotal.WithLabelValues("DENY", classificationResult.ActionType).Inc()
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		return c.JSON(http.StatusForbidden, ToolCallResponse{
			RequestID: requestID,
			Decision:  "DENY",
			Reason:    decisionResult.Reason,
		})
	case "REQUIRE_APPROVAL":
		approvalID := "ar_" + uuid.NewString()
		approval := models.ApprovalRequest{
			ApprovalRequestID: approvalID,
			TenantID:          tenant.TenantID,
			AgentID:           agentID,
			ToolID:            toolID,
			ActionType:        classificationResult.ActionType,
			RequestHash:       requestHash,
			Status:            "PENDING",
			ExpiresAt:         time.Now().UTC().Add(1 * time.Hour),
			CreatedAt:         time.Now().UTC(),
		}
		adr.ApprovalRequestID = approvalID
		d.Metrics.DecisionsTotal.WithLabelValues("REQUIRE_APPROVAL", classificationResult.ActionType).Inc()
		if err := d.Store.InsertApprovalRequest(c.Request().Context(), approval); err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist approval"})
		}
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}
		return c.JSON(http.StatusConflict, ToolCallResponse{
			RequestID:         requestID,
			Decision:          "REQUIRE_APPROVAL",
			Reason:            decisionResult.Reason,
			ApprovalRequestID: approvalID,
		})
	case "ALLOW":
		tool, err := d.Store.GetTool(c.Request().Context(), tenant.TenantID, toolID)
		if err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
		}

		if d.Connector == nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "connector not configured"})
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
		applyToolAuth(forwardHeaders, tool)

		toolStart := time.Now()
		resp, err := d.Connector.Execute(c.Request().Context(), connector.Request{
			Method:  payload.HTTPMethod,
			URL:     url,
			Headers: forwardHeaders,
			Body:    bodyBytes,
		})
		if err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "tool execution failed"})
		}
		d.Metrics.ToolLatencyMs.Observe(float64(time.Since(toolStart).Milliseconds()))
		adr.ResponseHash = resp.BodyHash
		if err := d.Store.InsertADR(c.Request().Context(), adr); err != nil {
			d.Metrics.ErrorsTotal.Inc()
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist decision"})
		}

		d.Metrics.DecisionsTotal.WithLabelValues("ALLOW", classificationResult.ActionType).Inc()
		d.Metrics.ToolExecTotal.Inc()

		return c.JSON(http.StatusOK, ToolCallResponse{
			RequestID:   requestID,
			Decision:    "ALLOW",
			Reason:      decisionResult.Reason,
			ToolStatus:  resp.Status,
			ToolHeaders: resp.Headers,
			ToolBody:    string(resp.Body),
		})
	default:
		d.Metrics.ErrorsTotal.Inc()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "unknown decision"})
	}
}

func (d Dependencies) handleEvidence(c *echo.Context) error {
	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := parsePositiveInt(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	tenantKey := c.Request().Header.Get(auth.TenantKeyHeader)
	agentID := c.Request().Header.Get(auth.AgentIDHeader)
	tenant, err := auth.AuthenticateTenant(c.Request().Context(), d.Store, tenantKey, agentID)
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
	for _, record := range records {
		exported = append(exported, models.ActionDecisionExport{
			DecisionID:        record.DecisionID,
			RequestID:         record.RequestID,
			TenantID:          record.TenantID,
			AgentID:           record.AgentID,
			ToolID:            record.ToolID,
			ActionType:        record.ActionType,
			ActionRisk:        record.ActionRisk,
			ActionSummary:     record.ActionSummary,
			Decision:          record.Decision,
			Reason:            record.Reason,
			RuleID:            record.RuleID,
			PolicyVersion:     record.PolicyVersion,
			RequestHash:       record.RequestHash,
			ResponseHash:      record.ResponseHash,
			ApprovalRequestID: record.ApprovalRequestID,
			Timestamp:         record.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, EvidenceResponse{
		TenantID: tenant.TenantID,
		Records:  exported,
	})
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

func applyToolAuth(headers map[string]string, tool models.Tool) {
	if tool.AuthType == "bearer" && tool.AuthValue != "" {
		headers["Authorization"] = "Bearer " + tool.AuthValue
	}
	if tool.AuthType == "api_key" && tool.AuthValue != "" {
		headers["X-Api-Key"] = tool.AuthValue
	}
}
