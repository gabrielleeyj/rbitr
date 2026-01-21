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
	adminKey, err := requireAdminScope(c, d.Store, "admin:write")
	if err != nil {
		return err
	}

	var payload AdminWriteLockRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	beforeLocked, _ := d.Store.GetAdminWriteLock(c.Request().Context())
	if err := d.Store.SetAdminWriteLock(c.Request().Context(), payload.Locked); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "update failed"})
	}
	if err := d.emitAuditEvent(c, adminKey, "", "SETTINGS.ADMIN_WRITE_LOCK.SET", "SETTINGS", "admin_write_lock", map[string]any{
		"admin_write_lock": beforeLocked,
	}, map[string]any{
		"admin_write_lock": payload.Locked,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":  "failed to audit write lock",
			"detail": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}
