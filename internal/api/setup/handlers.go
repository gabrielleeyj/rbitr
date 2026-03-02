package setup

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

type Dependencies struct {
	Service      Service
	AccessPolicy AccessPolicy
}

func RegisterRoutes(e *echo.Echo, deps *Dependencies) {
	if deps == nil || deps.Service == nil {
		return
	}

	e.GET("/setup/status", deps.handleStatus)
	e.POST("/setup/initialize", deps.handleInitialize)
}

func (d Dependencies) handleStatus(c *echo.Context) error {
	status, err := d.Service.Status(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load setup status"})
	}
	return c.JSON(http.StatusOK, status)
}

func (d Dependencies) handleInitialize(c *echo.Context) error {
	tokenFingerprint, authErr := d.AccessPolicy.Authorize(
		c.Request().Header.Get("Authorization"),
		c.RealIP(),
	)
	if authErr != nil {
		switch {
		case errors.Is(authErr, ErrSetupTokenMissing):
			c.Response().Header().Set("WWW-Authenticate", "Bearer")
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "setup token required"})
		case errors.Is(authErr, ErrSetupTokenInvalid):
			return c.JSON(http.StatusForbidden, map[string]string{"error": "invalid setup token"})
		case errors.Is(authErr, ErrSetupNetworkRejected):
			return c.JSON(http.StatusForbidden, map[string]string{"error": "setup request rejected by network policy"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "setup authorization failed"})
		}
	}

	idempotencyKey := strings.TrimSpace(c.Request().Header.Get(idempotencyHeader))
	if d.AccessPolicy.TokenRequired && idempotencyKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
	}

	var payload InitializeRequest
	decodeErr := json.NewDecoder(c.Request().Body).Decode(&payload)
	if decodeErr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	payload.IdempotencyKey = idempotencyKey
	payload.SetupTokenFingerprint = tokenFingerprint
	payload.ClientIP = strings.TrimSpace(c.RealIP())
	payload.UserAgent = strings.TrimSpace(c.Request().UserAgent())
	payload.RequestID = strings.TrimSpace(c.Request().Header.Get("X-Request-Id"))

	result, initErr := d.Service.Initialize(c.Request().Context(), &payload)
	if initErr != nil {
		switch {
		case errors.Is(initErr, ErrIdempotencyRequired):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
		case errors.Is(initErr, ErrInvalidRequest):
			fields := fieldErrorsFromError(initErr)
			if len(fields) > 0 {
				return c.JSON(http.StatusBadRequest, map[string]any{
					"error":  "invalid setup request",
					"fields": fields,
				})
			}
			return c.JSON(http.StatusBadRequest, map[string]string{"error": initErr.Error()})
		case errors.Is(initErr, ErrSchemaNotReady):
			return c.JSON(http.StatusPreconditionFailed, map[string]string{"error": "schema not ready; run migrations first"})
		case errors.Is(initErr, ErrSetupInProgress):
			return c.JSON(http.StatusConflict, map[string]string{"error": "setup already in progress"})
		case errors.Is(initErr, ErrIdempotencyConflict):
			return c.JSON(http.StatusConflict, map[string]string{"error": "idempotency key re-used with different payload"})
		case errors.Is(initErr, ErrSetupComplete):
			return c.JSON(http.StatusConflict, map[string]string{"error": "setup already completed"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "setup initialization failed"})
		}
	}

	return c.JSON(http.StatusCreated, result)
}
