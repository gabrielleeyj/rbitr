package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/ticketing"
)

type TicketingConfigRequest struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	ProjectKey string `json:"project_key"`
	IssueType  string `json:"issue_type"`
	AutoCreate bool   `json:"auto_create"`
}

type TicketingConfigResponse struct {
	TenantID          string    `json:"tenant_id"`
	Provider          string    `json:"provider"`
	Enabled           bool      `json:"enabled"`
	BaseURL           string    `json:"base_url"`
	SecretConfigured  bool      `json:"secret_configured"`
	ProjectKey        string    `json:"project_key"`
	IssueType         string    `json:"issue_type"`
	AutoCreate        bool      `json:"auto_create"`
	WebhookConfigured bool      `json:"webhook_secret_configured"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (d *Dependencies) handleTicketingConfigGet(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeTicketingRead); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	cfg, err := d.Store.GetTicketingConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusOK, TicketingConfigResponse{TenantID: tenantID})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load ticketing config"})
	}
	return c.JSON(http.StatusOK, TicketingConfigResponse{
		TenantID:          cfg.TenantID,
		Provider:          cfg.Provider,
		Enabled:           cfg.Enabled,
		BaseURL:           cfg.BaseURL,
		SecretConfigured:  cfg.SecretRef != "",
		ProjectKey:        cfg.ProjectKey,
		IssueType:         cfg.IssueType,
		AutoCreate:        cfg.AutoCreate,
		WebhookConfigured: cfg.WebhookSecretRef != "",
		CreatedAt:         cfg.CreatedAt,
		UpdatedAt:         cfg.UpdatedAt,
	})
}

func (d *Dependencies) handleTicketingConfigUpdate(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeTicketingWrite)
	if err != nil {
		return err
	}
	var payload TicketingConfigRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Provider != "" && payload.Provider != ticketing.ProviderJira &&
		payload.Provider != ticketing.ProviderServiceNow && payload.Provider != ticketing.ProviderLinear {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "provider must be jira, servicenow, or linear"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetTicketingConfig(c.Request().Context(), tenantID)
	cfg := models.TicketingConfig{
		TenantID:         tenantID,
		Provider:         payload.Provider,
		Enabled:          payload.Enabled,
		BaseURL:          payload.BaseURL,
		SecretRef:        before.SecretRef,
		ProjectKey:       payload.ProjectKey,
		IssueType:        payload.IssueType,
		AutoCreate:       payload.AutoCreate,
		WebhookSecretRef: before.WebhookSecretRef,
	}
	if err := d.Store.UpsertTicketingConfig(c.Request().Context(), &cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update ticketing config"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.TICKETING.CONFIG.UPDATE", "TENANT.TICKETING", tenantID, map[string]any{
		"provider":    before.Provider,
		"enabled":     before.Enabled,
		"auto_create": before.AutoCreate,
	}, map[string]any{
		"provider":    cfg.Provider,
		"enabled":     cfg.Enabled,
		"auto_create": cfg.AutoCreate,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to audit ticketing config update", "detail": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleTicketingSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeTicketingWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid secret_ref"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetTicketingConfig(c.Request().Context(), tenantID)
	cfg := before
	cfg.TenantID = tenantID
	cfg.SecretRef = payload.SecretRef

	if err := d.Store.UpsertTicketingConfig(c.Request().Context(), &cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update ticketing secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.TICKETING.SECRET_REF.SET", "TENANT.TICKETING", tenantID, map[string]any{
		"configured": before.SecretRef != "",
	}, map[string]any{
		"configured": true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to audit ticketing secret ref", "detail": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleTicketingWebhookSecretRefSet(c *echo.Context) error {
	adminKey, err := requireAdminScope(c, d.Store, scopeTicketingWrite)
	if err != nil {
		return err
	}
	var payload SecretRefRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !isValidSecretRef(payload.SecretRef) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid secret_ref"})
	}

	tenantID := c.Param("tenant_id")
	before, _ := d.Store.GetTicketingConfig(c.Request().Context(), tenantID)
	cfg := before
	cfg.TenantID = tenantID
	cfg.WebhookSecretRef = payload.SecretRef

	if err := d.Store.UpsertTicketingConfig(c.Request().Context(), &cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update webhook secret ref"})
	}
	if err := d.emitAuditEvent(c, adminKey, tenantID, "TENANT.TICKETING.WEBHOOK_SECRET_REF.SET", "TENANT.TICKETING", tenantID, map[string]any{
		"configured": before.WebhookSecretRef != "",
	}, map[string]any{
		"configured": true,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to audit webhook secret ref", "detail": err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (d *Dependencies) handleTicketingTest(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeTicketingTest); err != nil {
		return err
	}
	if d.TicketingService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "ticketing service not available"})
	}

	tenantID := c.Param("tenant_id")
	cfg, err := d.Store.GetTicketingConfig(c.Request().Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "ticketing not configured"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load ticketing config"})
	}
	if cfg.SecretRef == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ticketing secret not configured"})
	}

	testApproval := &models.ApprovalRequest{
		ApprovalRequestID: "test-approval-" + tenantID,
		TenantID:          tenantID,
		AgentID:           "test-agent",
		ToolID:            "test-tool",
		ActionType:        "test.action",
		Risk:              "MEDIUM",
		ActionSummary:     "Test ticket from rbitr",
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	}

	d.TicketingService.OnApprovalCreated(c.Request().Context(), tenantID, testApproval)
	return c.JSON(http.StatusOK, map[string]string{"status": "test ticket creation initiated"})
}

func (d *Dependencies) handleTicketLinksList(c *echo.Context) error {
	if _, err := requireAdminScope(c, d.Store, scopeTicketingRead); err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	links, err := d.Store.ListTicketLinks(c.Request().Context(), tenantID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list ticket links"})
	}
	if links == nil {
		links = []models.TicketLink{}
	}
	return c.JSON(http.StatusOK, links)
}

func (d *Dependencies) handleTicketingWebhook(c *echo.Context) error {
	if d.TicketingService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "ticketing service not available"})
	}

	provider := c.Param("provider")
	if provider != ticketing.ProviderJira && provider != ticketing.ProviderServiceNow && provider != ticketing.ProviderLinear {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported provider"})
	}

	const webhookMaxBytes = 65536
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, webhookMaxBytes))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to read body"})
	}

	externalKey, status := parseWebhookPayload(provider, body)
	if externalKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "could not extract ticket key from payload"})
	}

	action := ticketing.MapWebhookStatus(provider, status)
	if action == ticketing.ActionNone {
		return c.JSON(http.StatusOK, map[string]string{"status": "ignored"})
	}

	link, err := d.Store.GetTicketLinkByExternalKey(c.Request().Context(), provider, externalKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "ticket link not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to lookup ticket link"})
	}

	// Verify webhook signature if configured.
	cfg, cfgErr := d.Store.GetTicketingConfig(c.Request().Context(), link.TenantID)
	if cfgErr == nil && cfg.WebhookSecretRef != "" && d.SecretResolver != nil {
		secret, resolveErr := d.SecretResolver.Resolve(c.Request().Context(), cfg.WebhookSecretRef)
		if resolveErr != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve webhook secret"})
		}
		if !verifyWebhookSignature(c.Request(), body, secret) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
		}
	}

	ctx := c.Request().Context()
	decidedAt := time.Now().UTC()
	switch action {
	case ticketing.ActionApprove:
		err = d.Store.ApproveApprovalRequest(ctx, link.TenantID, link.ApprovalRequestID, "webhook:"+provider, "Approved via "+provider+" ticket "+externalKey, decidedAt)
	case ticketing.ActionDeny:
		err = d.Store.DenyApprovalRequest(ctx, link.TenantID, link.ApprovalRequestID, "webhook:"+provider, "Denied via "+provider+" ticket "+externalKey, decidedAt)
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidState) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "approval not actionable", "detail": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update approval"})
	}

	ticketStatus := ticketing.TicketStatusResolved
	if action == ticketing.ActionDeny {
		ticketStatus = ticketing.TicketStatusClosed
	}
	_ = d.Store.UpdateTicketLinkStatus(ctx, link.TicketLinkID, ticketStatus)

	return c.JSON(http.StatusOK, map[string]string{
		"status":              "processed",
		"approval_request_id": link.ApprovalRequestID,
		"action":              string(action),
	})
}

func parseWebhookPayload(provider string, body []byte) (externalKey, status string) {
	switch provider {
	case ticketing.ProviderJira:
		return parseJiraWebhook(body)
	case ticketing.ProviderServiceNow:
		return parseServiceNowWebhook(body)
	case ticketing.ProviderLinear:
		return parseLinearWebhook(body)
	default:
		return "", ""
	}
}

func parseJiraWebhook(body []byte) (externalKey, status string) {
	var payload struct {
		Issue struct {
			Key    string `json:"key"`
			Fields struct {
				Status struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"fields"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	return payload.Issue.Key, payload.Issue.Fields.Status.Name
}

func parseServiceNowWebhook(body []byte) (externalKey, status string) {
	var payload struct {
		Number string `json:"number"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	stateMap := map[string]string{
		"6": "resolved",
		"7": "closed",
		"8": "canceled",
	}
	mapped, ok := stateMap[payload.State]
	if !ok {
		mapped = payload.State
	}
	return payload.Number, mapped
}

func parseLinearWebhook(body []byte) (externalKey, status string) {
	var payload struct {
		Data struct {
			Identifier string `json:"identifier"`
			State      struct {
				Name string `json:"name"`
			} `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	return payload.Data.Identifier, payload.Data.State.Name
}

func verifyWebhookSignature(r *http.Request, body []byte, secret string) bool {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-Linear-Signature")
	}
	if signature == "" {
		return true
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
