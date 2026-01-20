package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

const (
	TenantKeyHeader = "X-Tenant-Key"
	AgentIDHeader   = "X-Agent-Id"
)

func AuthenticateTenant(ctx context.Context, st store.StoreAPI, tenantKey, agentID string) (models.Tenant, error) {
	if tenantKey == "" {
		return models.Tenant{}, ErrUnauthorized
	}
	if strings.TrimSpace(agentID) == "" {
		return models.Tenant{}, ErrForbidden
	}
	keyHash := utils.HashString(tenantKey)
	tenant, err := st.GetTenantByKeyHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.Tenant{}, ErrUnauthorized
		}
		return models.Tenant{}, err
	}
	return tenant, nil
}
