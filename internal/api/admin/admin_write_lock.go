package admin

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

type AdminWriteLockRequest struct {
	Locked bool `json:"locked"`
}

func (d Dependencies) handleAdminWriteLock(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	if err := requireAdminScope(c, d.Store); err != nil {
		return err
	}

	var payload AdminWriteLockRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if err := d.Store.SetAdminWriteLock(c.Request().Context(), payload.Locked); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "update failed"})
	}

	return c.NoContent(http.StatusNoContent)
}
