package setup

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
)

type Dependencies struct {
	Service Service
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
	var payload InitializeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	result, err := d.Service.Initialize(c.Request().Context(), payload)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRequest):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrSchemaNotReady):
			return c.JSON(http.StatusPreconditionFailed, map[string]string{"error": "schema not ready; run migrations first"})
		case errors.Is(err, ErrSetupComplete):
			return c.JSON(http.StatusConflict, map[string]string{"error": "setup already completed"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "setup initialization failed"})
		}
	}

	return c.JSON(http.StatusCreated, result)
}
