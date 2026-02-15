package auth

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidAgentID = errors.New("invalid agent_id")
)

const (
	TenantKeyHeader = "X-Tenant-Key"
	AgentIDHeader   = "X-Agent-Id"

	maxAgentIDLen = 128
)

var agentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// ValidateAgentID checks agent_id length and allowed charset.
func ValidateAgentID(agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ErrForbidden
	}
	if len(agentID) > maxAgentIDLen {
		return ErrInvalidAgentID
	}
	if !agentIDPattern.MatchString(agentID) {
		return ErrInvalidAgentID
	}
	return nil
}

// TenantKeyFromRequest resolves tenant authentication key from request headers.
// Authorization: Bearer <token> is preferred. X-Tenant-Key is supported as a
// temporary fallback unless explicitly disabled.
func TenantKeyFromRequest(r *http.Request, disableXTenantKey bool) (tenantKey string, usedFallback bool) {
	if r == nil {
		return "", false
	}
	if token := bearerToken(r.Header.Get(AuthorizationHeader)); token != "" {
		return token, false
	}
	if disableXTenantKey {
		return "", false
	}
	fallback := strings.TrimSpace(r.Header.Get(TenantKeyHeader))
	if fallback == "" {
		return "", false
	}
	return fallback, true
}

func AuthenticateTenant(ctx context.Context, st store.StoreAPI, tenantKey, agentID string) (models.Tenant, error) {
	if tenantKey == "" {
		return models.Tenant{}, ErrUnauthorized
	}
	if err := ValidateAgentID(agentID); err != nil {
		return models.Tenant{}, err
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
