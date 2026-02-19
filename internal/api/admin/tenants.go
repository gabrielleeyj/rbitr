package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

type CreateTenantRequest struct {
	Name string `json:"name"`
}

type CreateTenantResponse struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	APIKey   string `json:"api_key"`
	KeyID    string `json:"key_id"`
}

type SetTenantEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type TenantKeyIssueResponse struct {
	APIKey    string `json:"api_key"`
	KeyID     string `json:"key_id"`
	KeyPrefix string `json:"key_prefix"`
}

type tenantSoftDeleteStore interface {
	SoftDeleteTenant(ctx context.Context, tenantID string, deletedAt time.Time) error
}

func (d Dependencies) handleTenantCreate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeTenantsWrite)
	if err != nil {
		return err
	}
	var payload CreateTenantRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if payload.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	ctx := c.Request().Context()
	tenantID := "t_" + uuid.NewString()[:8]

	if err := d.Store.CreateTenant(ctx, tenantID, payload.Name); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create tenant"})
	}

	rawKey, keyHash, keyPrefix, err := utils.GenerateAPIKey()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate key"})
	}
	keyID := uuid.NewString()
	now := time.Now().UTC()
	if err := d.Store.CreateTenantKey(ctx, models.TenantKey{
		KeyID:     keyID,
		TenantID:  tenantID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		CreatedAt: now,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create tenant key"})
	}

	_ = d.emitAuditEvent(c, adminKey, tenantID, "TENANT.CREATED", "TENANT", tenantID, nil, map[string]any{
		"name":       payload.Name,
		"key_prefix": keyPrefix,
	})

	return c.JSON(http.StatusCreated, CreateTenantResponse{
		TenantID: tenantID,
		Name:     payload.Name,
		APIKey:   rawKey,
		KeyID:    keyID,
	})
}

func (d Dependencies) handleTenantKeysList(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	_, err := requireAdminScope(c, d.Store, scopeKeysRead)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	keys, err := d.Store.ListTenantKeys(c.Request().Context(), tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list keys"})
	}
	if keys == nil {
		keys = []models.TenantKey{}
	}
	return c.JSON(http.StatusOK, keys)
}

func (d Dependencies) handleTenantKeyCreate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeKeysRotate)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	ctx := c.Request().Context()
	now := time.Now().UTC()

	rawKey, keyHash, keyPrefix, err := utils.GenerateAPIKey()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate key"})
	}
	keyID := uuid.NewString()
	if err := d.Store.CreateTenantKey(ctx, models.TenantKey{
		KeyID:     keyID,
		TenantID:  tenantID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		CreatedAt: now,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create key"})
	}

	_ = d.emitAuditEvent(c, adminKey, tenantID, "TENANT.KEY.CREATED", "TENANT.KEY", keyID, nil, map[string]any{
		"key_prefix": keyPrefix,
	})

	return c.JSON(http.StatusCreated, TenantKeyIssueResponse{
		APIKey:    rawKey,
		KeyID:     keyID,
		KeyPrefix: keyPrefix,
	})
}

func (d Dependencies) handleTenantKeyRotate(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeKeysRotate)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)
	ctx := c.Request().Context()
	now := time.Now().UTC()

	// Revoke all existing active keys
	existingKeys, err := d.Store.ListTenantKeys(ctx, tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list keys"})
	}
	for _, k := range existingKeys {
		if k.RevokedAt == nil {
			_ = d.Store.RevokeTenantKey(ctx, tenantID, k.KeyID, now)
		}
	}

	// Create new key
	rawKey, keyHash, keyPrefix, err := utils.GenerateAPIKey()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate key"})
	}
	keyID := uuid.NewString()
	if err := d.Store.CreateTenantKey(ctx, models.TenantKey{
		KeyID:     keyID,
		TenantID:  tenantID,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		CreatedAt: now,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create key"})
	}

	_ = d.emitAuditEvent(c, adminKey, tenantID, "TENANT.KEY.ROTATED", "TENANT.KEY", keyID, nil, map[string]any{
		"key_prefix": keyPrefix,
	})

	return c.JSON(http.StatusOK, TenantKeyIssueResponse{
		APIKey:    rawKey,
		KeyID:     keyID,
		KeyPrefix: keyPrefix,
	})
}

func (d Dependencies) handleTenantKeyRevoke(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeKeysRevoke)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	keyID := c.Param("key_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	now := time.Now().UTC()
	if err := d.Store.RevokeTenantKey(c.Request().Context(), tenantID, keyID, now); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "key not found or already revoked"})
	}

	_ = d.emitAuditEvent(c, adminKey, tenantID, "TENANT.KEY.REVOKED", "TENANT.KEY", keyID, nil, map[string]any{
		"revoked_at": now,
	})

	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleTenantSetEnabled(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeTenantsWrite)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	var payload SetTenantEnabledRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if err := d.Store.SetTenantEnabled(c.Request().Context(), tenantID, payload.Enabled); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tenant not found"})
	}

	action := "TENANT.DISABLED"
	if payload.Enabled {
		action = "TENANT.ENABLED"
	}
	_ = d.emitAuditEvent(c, adminKey, tenantID, action, "TENANT", tenantID, nil, map[string]any{
		"enabled": payload.Enabled,
	})

	return c.NoContent(http.StatusNoContent)
}

func (d Dependencies) handleTenantDelete(c *echo.Context) error {
	if requestID := c.Request().Header.Get("X-Request-Id"); requestID != "" {
		c.Set(telemetry.CtxRequestID, requestID)
	}
	adminKey, err := requireAdminScope(c, d.Store, scopeTenantsWrite)
	if err != nil {
		return err
	}
	tenantID := c.Param("tenant_id")
	c.Set(telemetry.CtxTenantID, tenantID)

	deleter, ok := d.Store.(tenantSoftDeleteStore)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "tenant delete not supported"})
	}

	now := time.Now().UTC()
	if err := deleter.SoftDeleteTenant(c.Request().Context(), tenantID, now); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tenant not found"})
		}
		if errors.Is(err, store.ErrAdminWriteLocked) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "admin writes locked"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete tenant"})
	}

	_ = d.emitAuditEvent(c, adminKey, tenantID, "TENANT.DELETED", "TENANT", tenantID, map[string]any{
		"deleted_at": nil,
		"enabled":    true,
	}, map[string]any{
		"deleted_at": now,
		"enabled":    false,
	})

	return c.NoContent(http.StatusNoContent)
}
