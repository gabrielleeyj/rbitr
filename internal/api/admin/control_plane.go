package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/opa"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type PolicyCreateRequest struct {
	PolicyVersion string `json:"policy_version"`
	RegoModule    string `json:"rego_module"`
	Notes         string `json:"notes"`
}

type PolicyRollbackRequest struct {
	PolicyVersion string `json:"policy_version"`
}

type PolicySimulationRequest struct {
	PolicyVersion string         `json:"policy_version"`
	RegoModule    string         `json:"rego_module"`
	Input         map[string]any `json:"input"`
}

type ApprovalDecisionRequest struct {
	Comment string `json:"comment"`
}

type DefaultApprovalTTLRequest struct {
	Seconds int `json:"seconds"`
}

type PolicyVersionsResponse struct {
	TenantID            string                 `json:"tenant_id"`
	ActivePolicyVersion string                 `json:"active_policy_version"`
	Versions            []models.PolicyVersion `json:"versions"`
}

type SettingsResponse struct {
	AdminWriteLock            bool `json:"admin_write_lock"`
	DefaultApprovalTTLSeconds int  `json:"default_approval_ttl_seconds"`
}

type ToolResponse struct {
	ToolID   string `json:"tool_id"`
	TenantID string `json:"tenant_id"`
	BaseURL  string `json:"base_url"`
	AuthType string `json:"auth_type"`
	AuthSet  bool   `json:"auth_set"`
}

func (d Dependencies) handleTenantList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	items, err := d.Store.ListTenants(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tenants"})
	}
	return c.JSON(http.StatusOK, items)
}

func (d Dependencies) handleTenantDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	item, err := d.Store.GetTenant(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tenant not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d Dependencies) handleEvidenceList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	decision := c.QueryParam("decision")
	actionType := c.QueryParam("action_type")
	risk := c.QueryParam("risk")
	var since *time.Time
	if sinceParam := c.QueryParam("since"); sinceParam != "" {
		parsed, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid since"})
		}
		since = &parsed
	}

	records, err := d.Store.ListEvidenceFiltered(c.Request().Context(), tenantID, decision, actionType, risk, since, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list evidence"})
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
			if approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, record.ApprovalRequestID); err == nil {
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
	return c.JSON(http.StatusOK, map[string]any{"tenant_id": tenantID, "records": exported})
}

func (d Dependencies) handleApprovalsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	status := strings.ToUpper(strings.TrimSpace(c.QueryParam("status")))
	if status != "" && !isApprovalStatus(status) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid status"})
	}

	approvals, err := d.Store.ListApprovalRequests(c.Request().Context(), tenantID, status, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list approvals"})
	}
	now := time.Now().UTC()
	for i := range approvals {
		if approvalExpired(&approvals[i], now) {
			if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvals[i].ApprovalRequestID, now); err == nil {
				approvals[i].Status = "EXPIRED"
				approvals[i].DecidedAt = &now
			}
		}
	}
	return c.JSON(http.StatusOK, approvals)
}

func (d Dependencies) handleApprovalDetail(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	approvalID := c.Param("approval_request_id")
	approval, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	now := time.Now().UTC()
	if approvalExpired(&approval, now) {
		if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); err == nil {
			approval.Status = "EXPIRED"
			approval.DecidedAt = &now
		}
	}
	return c.JSON(http.StatusOK, approval)
}

func (d Dependencies) handleApprovalApprove(c *echo.Context) error {
	return d.handleApprovalDecision(c, "APPROVED", "APPROVAL.REQUEST.APPROVE")
}

func (d Dependencies) handleApprovalDeny(c *echo.Context) error {
	return d.handleApprovalDecision(c, "DENIED", "APPROVAL.REQUEST.DENY")
}

func (d Dependencies) handleApprovalRevoke(c *echo.Context) error {
	return d.handleApprovalDecision(c, "REVOKED", "APPROVAL.REQUEST.REVOKE")
}

func (d Dependencies) handlePolicyVersions(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	versions, err := d.Store.ListPolicyVersions(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list policies"})
	}
	config, err := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load tenant config"})
	}

	response := PolicyVersionsResponse{
		TenantID:            tenantID,
		ActivePolicyVersion: config.ActivePolicyVersion,
		Versions:            versions,
	}
	return c.JSON(http.StatusOK, response)
}

func (d Dependencies) handlePolicyVersionGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	version := c.Param("policy_version")
	item, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load policy"})
	}
	return c.JSON(http.StatusOK, item)
}

func (d Dependencies) handlePolicyCreate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload PolicyCreateRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.PolicyVersion == "" || payload.RegoModule == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "policy_version and rego_module required"})
	}
	if err := validateRegoModule(payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if _, err := opa.PrepareQuery(c.Request().Context(), payload.RegoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":  "rego compilation failed",
			"detail": err.Error(),
		})
	}

	tenantID := c.Param("tenant_id")
	if err := d.Store.CreatePolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion, payload.RegoModule, adminKey.AdminKeyID, payload.Notes); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create policy"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.CREATE", "POLICY.VERSION", payload.PolicyVersion, nil, map[string]any{
		"policy_version": payload.PolicyVersion,
		"created_by":     adminKey.AdminKeyID,
		"notes":          payload.Notes,
		"rego_sha256":    utils.HashString(payload.RegoModule),
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy create",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusCreated)
}

func (d Dependencies) handlePolicyPublish(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	version := c.Param("policy_version")
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.Store.PublishPolicyVersion(c.Request().Context(), tenantID, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to publish policy"})
	}
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.PUBLISH", "POLICY.ACTIVE", version, map[string]any{
		"active_policy_version": before.ActivePolicyVersion,
	}, map[string]any{
		"active_policy_version": after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy publish",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handlePolicyRollback(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload PolicyRollbackRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)
	if err := d.Store.RollbackPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to rollback policy"})
	}
	after, _ := d.Store.GetTenantConfig(c.Request().Context(), tenantID)

	target := payload.PolicyVersion
	if target == "" {
		target = after.ActivePolicyVersion
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "POLICY.VERSION.ROLLBACK", "POLICY.ACTIVE", target, map[string]any{
		"active_policy_version": before.ActivePolicyVersion,
	}, map[string]any{
		"active_policy_version": after.ActivePolicyVersion,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit policy rollback",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handlePolicySimulate(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	var payload PolicySimulationRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Input == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "input required"})
	}

	tenantID := c.Param("tenant_id")
	regoModule := payload.RegoModule
	if regoModule == "" {
		if payload.PolicyVersion != "" {
			version, err := d.Store.GetPolicyVersion(c.Request().Context(), tenantID, payload.PolicyVersion)
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "policy version not found"})
			}
			regoModule = version.RegoModule
		} else {
			policy, err := d.Store.GetPolicy(c.Request().Context(), tenantID)
			if err != nil {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "active policy not found"})
			}
			regoModule = policy.RegoModule
		}
	}
	if err := validateRegoModule(regoModule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	engine := opa.NewEngine(regoModule)
	result, err := engine.Evaluate(c.Request().Context(), payload.Input)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":  "policy evaluation failed",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"decision": map[string]any{
			"version":     result.Version,
			"decision":    result.Decision,
			"risk":        result.Risk,
			"rule":        result.Rule,
			"reasons":     result.Reasons,
			"constraints": result.Constraints,
			"tags":        result.Tags,
		},
	})
}

func (d Dependencies) handleRiskOverridesList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	overrides, err := d.Store.ListRiskOverrides(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list overrides"})
	}
	return c.JSON(http.StatusOK, overrides)
}

func (d Dependencies) handleRiskOverrideDelete(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	actionType := c.Param("action_type")
	beforeRisk, _ := d.Store.GetRiskOverride(c.Request().Context(), tenantID, actionType)
	if err := d.Store.DeleteRiskOverride(c.Request().Context(), tenantID, actionType); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete override"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "RISK_OVERRIDE.DELETE", "RISK_OVERRIDE", actionType, map[string]any{
		"action_type": actionType,
		"action_risk": beforeRisk,
	}, nil); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit override delete",
			"detail": err.Error(),
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleToolsList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}

	tenantID := c.Param("tenant_id")
	tools, err := d.Store.ListTools(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list tools"})
	}
	response := make([]ToolResponse, 0, len(tools))
	for _, tool := range tools {
		response = append(response, ToolResponse{
			ToolID:   tool.ToolID,
			TenantID: tool.TenantID,
			BaseURL:  tool.BaseURL,
			AuthType: tool.AuthType,
			AuthSet:  tool.AuthValue != "",
		})
	}
	return c.JSON(http.StatusOK, response)
}

func (d Dependencies) handleSettingsGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	locked, err := d.Store.GetAdminWriteLock(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
	}
	defaultTTL := 900
	if value, err := d.Store.GetDefaultApprovalTTLSeconds(c.Request().Context()); err == nil && value > 0 {
		defaultTTL = value
	}
	return c.JSON(http.StatusOK, SettingsResponse{
		AdminWriteLock:            locked,
		DefaultApprovalTTLSeconds: defaultTTL,
	})
}

func (d Dependencies) handleAuditList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	tenantID := c.Param("tenant_id")
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	events, err := d.Store.ListAuditEvents(c.Request().Context(), tenantID, limit, offset, action, resourceType, actorID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to list audit events",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d Dependencies) handleAuditListAll(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, "admin:read"); err != nil {
		return err
	}
	limit, err := parseLimit(c.QueryParam("limit"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid limit"})
	}
	offset, err := parseOffset(c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid offset"})
	}
	action := c.QueryParam("action")
	resourceType := c.QueryParam("resource_type")
	actorID := c.QueryParam("actor_id")
	events, err := d.Store.ListAuditEvents(c.Request().Context(), "", limit, offset, action, resourceType, actorID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to list audit events",
			"detail": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, events)
}

func (d Dependencies) handleDefaultApprovalTTLUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload DefaultApprovalTTLRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if payload.Seconds < 60 || payload.Seconds > 86400 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "seconds must be between 60 and 86400"})
	}

	beforeTTL, _ := d.Store.GetDefaultApprovalTTLSeconds(c.Request().Context())
	if err := d.Store.SetDefaultApprovalTTLSeconds(c.Request().Context(), payload.Seconds); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update default approval ttl"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.APPROVAL_TTL_DEFAULT.SET", "SETTINGS", "default_approval_ttl_seconds", map[string]any{
		"value": beforeTTL,
	}, map[string]any{
		"value": payload.Seconds,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit approval ttl update",
			"detail": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleApprovalDecision(c *echo.Context, status, auditAction string) error {
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload ApprovalDecisionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	tenantID := c.Param("tenant_id")
	approvalID := c.Param("approval_request_id")
	before, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	now := time.Now().UTC()
	if approvalExpired(&before, now) {
		if err := d.Store.MarkApprovalExpired(c.Request().Context(), tenantID, approvalID, now); err == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval expired"})
		}
	}

	decidedAt := time.Now().UTC()
	switch status {
	case "APPROVED":
		err = d.Store.ApproveApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case "DENIED":
		err = d.Store.DenyApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	case "REVOKED":
		err = d.Store.RevokeApprovalRequest(c.Request().Context(), tenantID, approvalID, adminKey.AdminKeyID, payload.Comment, decidedAt)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid status"})
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "approval not found"})
		}
		if errors.Is(err, store.ErrInvalidState) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval state invalid"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}

	after, err := d.Store.GetApprovalRequest(c.Request().Context(), tenantID, approvalID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
	}
	if d.Metrics != nil {
		switch status {
		case "APPROVED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("approved").Inc()
		case "DENIED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("denied").Inc()
		case "REVOKED":
			d.Metrics.ApprovalsResolvedTotal.WithLabelValues("revoked").Inc()
		}
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, auditAction, "APPROVAL.REQUEST", approvalID, map[string]any{
		"status":           before.Status,
		"decided_at":       before.DecidedAt,
		"decided_by":       before.DecidedBy,
		"decision_comment": before.DecisionComment,
	}, map[string]any{
		"status":           after.Status,
		"decided_at":       after.DecidedAt,
		"decided_by":       after.DecidedBy,
		"decision_comment": after.DecisionComment,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit approval decision",
			"detail": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, after)
}

func (d Dependencies) emitAuditEvent(c *echo.Context, adminKey models.AdminKey, tenantID, action, resourceType, resourceID string, before, after map[string]any) error {
	action = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, action)
	resourceType = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, resourceType)
	action = strings.ToUpper(strings.TrimSpace(action))
	resourceType = strings.ToUpper(strings.TrimSpace(resourceType))
	resourceID = strings.TrimSpace(resourceID)
	if action == "" || resourceType == "" {
		return errors.New("audit event action and resource type required")
	}
	if !regexp.MustCompile(`^[A-Z0-9_]+(\.[A-Z0-9_]+)*$`).MatchString(action) {
		return errors.New("audit event action violates format constraint")
	}
	beforeJSON, err := marshalAuditPayload(before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalAuditPayload(after)
	if err != nil {
		return err
	}
	event := models.AdminAuditEvent{
		AuditEventID: "ae_" + uuid.NewString(),
		TenantID:     tenantID,
		ActorType:    "admin_key",
		ActorID:      adminKey.AdminKeyID,
		ActorDisplay: adminKey.AdminKeyID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       beforeJSON,
		After:        afterJSON,
		RequestID:    c.Request().Header.Get("X-Request-Id"),
		IP:           c.RealIP(),
		UserAgent:    c.Request().UserAgent(),
		CreatedAt:    time.Now().UTC(),
	}
	return d.Store.InsertAuditEvent(c.Request().Context(), event)
}

func marshalAuditPayload(payload map[string]any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func isApprovalStatus(value string) bool {
	switch value {
	case "PENDING", "APPROVED", "DENIED", "EXECUTED", "EXPIRED", "REVOKED":
		return true
	default:
		return false
	}
}

func approvalExpired(approval *models.ApprovalRequest, now time.Time) bool {
	if approval == nil {
		return false
	}
	if approval.Status != "PENDING" && approval.Status != "APPROVED" {
		return false
	}
	return now.After(approval.ExpiresAt)
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return 50, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

func parseOffset(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid")
	}
	return parsed, nil
}

func validateRegoModule(module string) error {
	if !strings.Contains(module, "import rego.v1") {
		return errors.New("rego module must import rego.v1")
	}
	if !strings.Contains(module, "package rbitr.policy") {
		return errors.New("rego module must define package rbitr.policy")
	}
	return nil
}
