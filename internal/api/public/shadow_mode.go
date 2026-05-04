package public

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/mcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
)

const (
	enforcementModeEnforce = "enforce"
	enforcementModeShadow  = "shadow"
	restShadowDenyHeader   = "X-Rbitr-Shadow-Deny"
	mcpClientTimeout       = 30 * time.Second
)

func normalizeEnforcementMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case enforcementModeShadow:
		return enforcementModeShadow
	default:
		return enforcementModeEnforce
	}
}

func (d *Dependencies) tenantEnforcementMode(ctx context.Context, tenantID string) (string, error) {
	if !d.Config.FeatureShadowMode || d.Store == nil {
		return enforcementModeEnforce, nil
	}
	tenantConfig, err := d.Store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return enforcementModeEnforce, nil
		}
		return "", err
	}
	return normalizeEnforcementMode(tenantConfig.EnforcementMode), nil
}

func isShadowMode(mode string) bool {
	return normalizeEnforcementMode(mode) == enforcementModeShadow
}

func buildShadowDecisionMetadata(ruleID, risk, policyVersion string, reasons []models.DecisionReason, constraints map[string]any) map[string]any {
	meta := map[string]any{
		"mode":              enforcementModeShadow,
		"enforced":          false,
		"original_decision": string(decisionDeny),
		fieldReasons:        decisionReasonsAsMaps(reasons),
	}
	if ruleID != "" {
		meta["rule_id"] = ruleID
	}
	if risk != "" {
		meta["risk"] = risk
	}
	if policyVersion != "" {
		meta[fieldPolicyVersion] = policyVersion
	}
	if len(constraints) > 0 {
		meta["constraints"] = constraints
	}
	return meta
}

func (d *Dependencies) executeRESTShadowDeny(
	c *echo.Context,
	tenantID string,
	toolID string,
	requestID string,
	payload ToolCallRequest,
	bodyBytes []byte,
	filteredHeaders map[string]string,
	reasons []models.DecisionReason,
	shadowMeta map[string]any,
) error {
	toolStart := time.Now()
	resp, err := d.executeToolCall(c.Request().Context(), tenantID, toolID, payload, bodyBytes, filteredHeaders)
	if err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		if errors.Is(err, errToolNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": errToolNotFoundMessage})
		}
		if errors.Is(err, errConnectorMissing) {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "connector not configured"})
		}
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "tool execution failed"})
	}
	if d.Metrics != nil {
		d.Metrics.ToolLatencyMs.Observe(float64(time.Since(toolStart).Milliseconds()))
		d.Metrics.ToolExecTotal.Inc()
	}
	c.Response().Header().Set(restShadowDenyHeader, "true")
	return c.JSON(http.StatusOK, ToolCallResponse{
		RequestID:    requestID,
		Decision:     string(decisionDeny),
		Reason:       firstReasonMessage(reasons),
		ToolStatus:   resp.Status,
		ToolHeaders:  resp.Headers,
		ToolBody:     string(resp.Body),
		RbitrShadow:  shadowMeta,
		ShadowDenied: true,
	})
}

func (d *Dependencies) executeMCPShadowDeny(
	ctx context.Context,
	tool *models.Tool,
	forwardReq *mcp.Request,
	shadowMeta map[string]any,
) (*mcp.Response, error) {
	if tool == nil || tool.MCPUpstreamURL == "" {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, mcp.NewInternalError("tool not configured for MCP")), nil
	}

	mcpClient := connector.NewMCPClient(mcpClientTimeout)
	toolStart := time.Now()
	upstreamResp, err := mcpClient.ForwardRequest(ctx, tool.MCPUpstreamURL, forwardReq)
	if d.Metrics != nil {
		d.Metrics.ToolLatencyMs.Observe(float64(time.Since(toolStart).Milliseconds()))
	}
	if err != nil {
		if d.Metrics != nil {
			d.Metrics.ErrorsTotal.Inc()
		}
		return mcp.NewErrorResponse(forwardReq.ID, &mcp.ErrorObject{
			Code:    mcp.ErrorInternalError,
			Message: errUpstreamToolExecFailed,
		}), nil
	}

	upstreamResp.Result = withMCPShadowMetadata(upstreamResp.Result, shadowMeta)
	if d.Metrics != nil {
		d.Metrics.ToolExecTotal.Inc()
	}
	return upstreamResp, nil
}

func withMCPShadowMetadata(raw json.RawMessage, shadowMeta map[string]any) json.RawMessage {
	if len(raw) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), jsonNull) {
		out, _ := json.Marshal(map[string]any{
			"_rbitr_shadow": shadowMeta,
		})
		return out
	}

	var objectResult map[string]any
	if err := json.Unmarshal(raw, &objectResult); err == nil {
		objectResult["_rbitr_shadow"] = shadowMeta
		out, marshalErr := json.Marshal(objectResult)
		if marshalErr == nil {
			return out
		}
	}

	var original any
	if err := json.Unmarshal(raw, &original); err != nil {
		original = string(raw)
	}
	out, _ := json.Marshal(map[string]any{
		"_rbitr_result": original,
		"_rbitr_shadow": shadowMeta,
	})
	return out
}
