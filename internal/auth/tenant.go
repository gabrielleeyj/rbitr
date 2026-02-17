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

type TenantAuthResult struct {
	Tenant             models.Tenant
	LegacyHashMatched  bool
	LegacyHashUpgraded bool
}

type tenantKeyHashUpgrader interface {
	UpgradeTenantKeyHash(ctx context.Context, oldKeyHash, newKeyHash string) error
}

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
	result, err := AuthenticateTenantDetailed(ctx, st, tenantKey, agentID)
	if err != nil {
		return models.Tenant{}, err
	}
	return result.Tenant, nil
}

func AuthenticateTenantDetailed(ctx context.Context, st store.StoreAPI, tenantKey, agentID string) (TenantAuthResult, error) {
	if tenantKey == "" {
		return TenantAuthResult{}, ErrUnauthorized
	}
	if err := ValidateAgentID(agentID); err != nil {
		return TenantAuthResult{}, err
	}
	candidates := utils.TenantKeyHashCandidatesFromEnv(tenantKey)
	return authenticateTenantWithHashCandidates(ctx, st, candidates)
}

func authenticateTenantWithHashCandidates(
	ctx context.Context,
	st store.StoreAPI,
	candidates utils.TenantKeyHashCandidates,
) (TenantAuthResult, error) {
	lookup := func(hash string) (models.Tenant, error) {
		if strings.TrimSpace(hash) == "" {
			return models.Tenant{}, store.ErrNotFound
		}
		return st.GetTenantByKeyHash(ctx, hash)
	}

	tryLookup := func(hash string) (models.Tenant, bool, error) {
		tenant, err := lookup(hash)
		if err == nil {
			return tenant, true, nil
		}
		if errors.Is(err, store.ErrNotFound) {
			return models.Tenant{}, false, nil
		}
		return models.Tenant{}, false, err
	}

	if tenant, matched, err := tryLookup(candidates.Current); err != nil {
		return TenantAuthResult{}, err
	} else if matched {
		return TenantAuthResult{Tenant: tenant}, nil
	}

	for _, previousHash := range candidates.Previous {
		tenant, matched, err := tryLookup(previousHash)
		if err != nil {
			return TenantAuthResult{}, err
		}
		if matched {
			return TenantAuthResult{Tenant: tenant}, nil
		}
	}

	tenant, matched, err := tryLookup(candidates.Legacy)
	if err != nil {
		return TenantAuthResult{}, err
	}
	if !matched {
		return TenantAuthResult{}, ErrUnauthorized
	}

	result := TenantAuthResult{
		Tenant:            tenant,
		LegacyHashMatched: true,
	}
	if strings.TrimSpace(candidates.Current) == "" || candidates.Current == candidates.Legacy {
		return result, nil
	}
	upgrader, ok := st.(tenantKeyHashUpgrader)
	if !ok {
		return result, nil
	}
	if err := upgrader.UpgradeTenantKeyHash(ctx, candidates.Legacy, candidates.Current); err == nil {
		result.LegacyHashUpgraded = true
	}
	return result, nil
}
