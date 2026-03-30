package aws

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	"github.com/labstack/echo/v5"
)

// ActivationHandler handles the AWS Marketplace customer registration flow.
// When a customer subscribes via AWS Marketplace, they are redirected to the
// rbitr landing page with a registration token. This handler resolves the token
// to a customer identifier.
type ActivationHandler struct {
	meteringClient MeteringClient
	provider       *Provider
	reporter       *Reporter
}

// NewActivationHandler creates a handler for the AWS Marketplace activation endpoint.
func NewActivationHandler(meteringClient MeteringClient, provider *Provider, reporter *Reporter) *ActivationHandler {
	return &ActivationHandler{
		meteringClient: meteringClient,
		provider:       provider,
		reporter:       reporter,
	}
}

type resolveTokenRequest struct {
	RegistrationToken string `json:"registration_token"`
}

type resolveTokenResponse struct {
	CustomerID         string `json:"customer_id"`
	CustomerAWSAccount string `json:"customer_aws_account_id"`
	ProductCode        string `json:"product_code"`
}

// HandleResolveToken resolves an AWS Marketplace registration token to a customer ID.
func (h *ActivationHandler) HandleResolveToken(c *echo.Context) error {
	var req resolveTokenRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.RegistrationToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "registration_token is required",
		})
	}

	ctx := c.Request().Context()

	output, err := h.meteringClient.ResolveCustomer(ctx, &marketplacemetering.ResolveCustomerInput{
		RegistrationToken: &req.RegistrationToken,
	})
	if err != nil {
		slog.Error("aws marketplace: ResolveCustomer failed", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("failed to resolve registration token: %v", err),
		})
	}

	customerID := derefString(output.CustomerIdentifier)
	if customerID == "" {
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": "AWS returned empty customer identifier",
		})
	}

	// Update the provider and reporter with the resolved customer ID.
	if h.provider != nil {
		if err := h.provider.SetCustomerID(ctx, customerID); err != nil {
			slog.Warn("aws marketplace: entitlement refresh after activation failed", "error", err)
		}
	}
	if h.reporter != nil {
		h.reporter.SetCustomerID(customerID)
	}

	slog.Info("aws marketplace: customer activated",
		"customer_id", customerID,
		"product_code", derefString(output.ProductCode),
	)

	return c.JSON(http.StatusOK, resolveTokenResponse{
		CustomerID:         customerID,
		CustomerAWSAccount: derefString(output.CustomerAWSAccountId),
		ProductCode:        derefString(output.ProductCode),
	})
}

// RegisterRoutes registers the AWS Marketplace activation endpoint on the given Echo instance.
func RegisterRoutes(e *echo.Echo, handler *ActivationHandler) {
	e.POST("/api/marketplace/aws/resolve-token", handler.HandleResolveToken)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
