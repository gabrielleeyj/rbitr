package azure

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
)

// WebhookHandler handles Azure Marketplace SaaS lifecycle events:
// landing page token resolution and webhook notifications.
type WebhookHandler struct {
	provider          *Provider
	reporter          *Reporter
	fulfillmentClient FulfillmentClient
}

// NewWebhookHandler creates a handler for Azure Marketplace endpoints.
func NewWebhookHandler(provider *Provider, reporter *Reporter, fulfillClient FulfillmentClient) *WebhookHandler {
	return &WebhookHandler{
		provider:          provider,
		reporter:          reporter,
		fulfillmentClient: fulfillClient,
	}
}

// landingRequest is the query parameter or body for the landing page.
type landingRequest struct {
	Token string `query:"token" json:"token"`
}

// webhookPayload is the Azure Marketplace webhook notification body.
type webhookPayload struct {
	Action         string `json:"action"`
	ActivityID     string `json:"activityId"`
	OfferID        string `json:"offerId"`
	PlanID         string `json:"planId"`
	Quantity       int    `json:"quantity"`
	SubscriptionID string `json:"subscriptionId"`
	Status         string `json:"status"`
	OperationID    string `json:"id"`
}

// HandleLanding resolves an Azure Marketplace purchase token from the landing
// page redirect, activates the subscription, and configures the provider.
func (h *WebhookHandler) HandleLanding(c *echo.Context) error {
	var req landingRequest
	if err := c.Bind(&req); err != nil || req.Token == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "marketplace token is required",
		})
	}

	ctx := c.Request().Context()

	resolved, err := h.fulfillmentClient.ResolveToken(ctx, req.Token)
	if err != nil {
		slog.Error("azure marketplace: resolve token failed", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "failed to resolve marketplace token",
		})
	}

	if resolved.SubscriptionID == "" {
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "Azure returned empty subscription ID",
		})
	}

	// Activate the subscription with the resolved plan.
	planID := resolved.PlanID
	if err := h.fulfillmentClient.ActivateSubscription(ctx, resolved.SubscriptionID, planID); err != nil {
		slog.Error("azure marketplace: activate subscription failed",
			"subscription_id", resolved.SubscriptionID,
			"error", err,
		)
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "failed to activate subscription",
		})
	}

	// Update provider and reporter with the resolved subscription.
	if err := h.provider.SetSubscription(ctx, resolved.SubscriptionID, planID); err != nil {
		slog.Warn("azure marketplace: subscription refresh after activation failed", "error", err)
	}
	h.reporter.SetSubscription(resolved.SubscriptionID, planID)

	slog.Info("azure marketplace: subscription activated",
		"subscription_id", resolved.SubscriptionID,
		"plan_id", planID,
	)

	return c.JSON(http.StatusOK, map[string]string{
		"subscription_id": resolved.SubscriptionID,
		"plan_id":         planID,
		"status":          "activated",
	})
}

// HandleWebhook processes Azure Marketplace lifecycle webhook notifications.
// Azure requires a 200 response within 10 seconds.
func (h *WebhookHandler) HandleWebhook(c *echo.Context) error {
	var payload webhookPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid webhook payload",
		})
	}

	slog.Info("azure marketplace: webhook received",
		"action", payload.Action,
		"subscription_id", payload.SubscriptionID,
		"plan_id", payload.PlanID,
		"operation_id", payload.OperationID,
	)

	ctx := c.Request().Context()

	switch payload.Action {
	case "ChangePlan":
		h.provider.UpdateStatus(ctx, StatusSubscribed, payload.PlanID)
		h.reporter.SetSubscription(payload.SubscriptionID, payload.PlanID)

	case "Suspend":
		h.provider.UpdateStatus(ctx, StatusSuspended, "")

	case "Reinstate":
		h.provider.UpdateStatus(ctx, StatusSubscribed, "")

	case "Unsubscribe":
		h.provider.UpdateStatus(ctx, StatusUnsubscribed, "")

	default:
		slog.Warn("azure marketplace: unhandled webhook action", "action", payload.Action)
	}

	// Azure requires 200 OK acknowledgement.
	return c.NoContent(http.StatusOK)
}

// RegisterRoutes registers the Azure Marketplace endpoints on the given Echo instance.
func RegisterRoutes(e *echo.Echo, handler *WebhookHandler) {
	e.GET("/api/marketplace/azure/landing", handler.HandleLanding)
	e.POST("/api/marketplace/azure/webhook", handler.HandleWebhook)
}
