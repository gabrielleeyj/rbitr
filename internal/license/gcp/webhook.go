package gcp

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
)

// WebhookHandler handles GCP Marketplace Pub/Sub push notifications for
// entitlement lifecycle state changes.
type WebhookHandler struct {
	provider          *Provider
	procurementClient ProcurementClient
}

// NewWebhookHandler creates a handler for GCP Marketplace Pub/Sub webhook events.
func NewWebhookHandler(provider *Provider, procClient ProcurementClient) *WebhookHandler {
	return &WebhookHandler{
		provider:          provider,
		procurementClient: procClient,
	}
}

// pubSubMessage is the outer Pub/Sub push message envelope.
type pubSubMessage struct {
	Message struct {
		Attributes map[string]string `json:"attributes"`
		Data       json.RawMessage   `json:"data"`
		MessageID  string            `json:"messageId"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// HandleWebhook processes a Pub/Sub push notification for entitlement changes.
func (h *WebhookHandler) HandleWebhook(c *echo.Context) error {
	var msg pubSubMessage
	if err := c.Bind(&msg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid Pub/Sub message",
		})
	}

	eventType := msg.Message.Attributes["event_type"]
	entitlementID := msg.Message.Attributes["entitlement_id"]

	slog.Info("gcp marketplace: webhook received",
		"event_type", eventType,
		"entitlement_id", entitlementID,
		"message_id", msg.Message.MessageID,
	)

	ctx := c.Request().Context()

	switch eventType {
	case "ENTITLEMENT_CREATION_REQUESTED":
		h.handleActivationRequest(entitlementID)

	case activeState:
		h.provider.UpdateEntitlement(ctx, entitlementID, activeState)

	case cancelledState:
		h.provider.UpdateEntitlement(ctx, entitlementID, cancelledState)

	case pendingCancellationState:
		h.provider.UpdateEntitlement(ctx, entitlementID, pendingCancellationState)

	case "ENTITLEMENT_PLAN_CHANGE_REQUESTED":
		h.handlePlanChangeRequest(entitlementID)

	case "ENTITLEMENT_PLAN_CHANGED":
		h.provider.UpdateEntitlement(ctx, entitlementID, activeState)

	default:
		slog.Warn("gcp marketplace: unhandled webhook event type", "event_type", eventType)
	}

	// Always return 200 to acknowledge the Pub/Sub message.
	// Returning non-200 causes Pub/Sub to redeliver.
	return c.NoContent(http.StatusOK)
}

// handleActivationRequest auto-approves entitlement activation requests.
func (h *WebhookHandler) handleActivationRequest(entitlementID string) {
	if entitlementID == "" {
		slog.Warn("gcp marketplace: activation request missing entitlement ID")
		return
	}

	name := "providers/" + h.provider.providerID + "/entitlements/" + entitlementID
	if err := h.procurementClient.ApproveEntitlement(name); err != nil {
		slog.Error("gcp marketplace: failed to approve entitlement",
			"entitlement_id", entitlementID,
			"error", err,
		)
		return
	}

	slog.Info("gcp marketplace: entitlement approved", "entitlement_id", entitlementID)
}

// handlePlanChangeRequest auto-approves plan change requests.
// In production, you may want to validate the new plan before approving.
func (h *WebhookHandler) handlePlanChangeRequest(entitlementID string) {
	slog.Info("gcp marketplace: plan change requested (auto-approved)",
		"entitlement_id", entitlementID,
	)
	// Plan changes are auto-approved by GCP unless the provider explicitly rejects.
	// We log the event and let the next refresh pick up the new plan.
}

// RegisterRoutes registers the GCP Marketplace webhook endpoint on the given Echo instance.
func RegisterRoutes(e *echo.Echo, handler *WebhookHandler) {
	e.POST("/api/marketplace/gcp/webhook", handler.HandleWebhook)
}
