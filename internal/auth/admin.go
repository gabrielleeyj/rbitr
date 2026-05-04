package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

const (
	AdminKeyHeader      = "X-Admin-Key"
	AuthorizationHeader = "Authorization"

	scopeAdminRead  = "admin:read"
	scopeAdminWrite = "admin:write"
)

func AdminKeyFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := bearerToken(r.Header.Get(AuthorizationHeader)); token != "" {
		return token
	}
	return r.Header.Get(AdminKeyHeader)
}

//nolint:mnd // ignore spliting of token.
func bearerToken(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

type adminKeyHashUpgrader interface {
	UpgradeAdminKeyHash(ctx context.Context, oldKeyHash, newKeyHash string) error
}

func AuthenticateAdmin(ctx context.Context, st store.StoreAPI, adminKey, requiredScope string) (models.AdminKey, error) {
	key, err := AuthenticateAdminAny(ctx, st, adminKey)
	if err != nil {
		return models.AdminKey{}, err
	}
	if !hasScope(key.Scopes, requiredScope) {
		return models.AdminKey{}, ErrForbidden
	}
	return key, nil
}

func AuthenticateAdminAny(ctx context.Context, st store.StoreAPI, adminKey string) (models.AdminKey, error) {
	if adminKey == "" {
		return models.AdminKey{}, ErrUnauthorized
	}
	candidates := utils.AdminKeyHashCandidatesFromEnv(adminKey)
	return authenticateAdminWithHashCandidates(ctx, st, candidates)
}

func authenticateAdminWithHashCandidates(
	ctx context.Context,
	st store.StoreAPI,
	candidates utils.AdminKeyHashCandidates,
) (models.AdminKey, error) {
	lookup := func(hash string) (models.AdminKey, error) {
		if strings.TrimSpace(hash) == "" {
			return models.AdminKey{}, store.ErrNotFound
		}
		return st.GetAdminKeyByHash(ctx, hash)
	}

	tryLookup := func(hash string) (models.AdminKey, bool, error) {
		key, err := lookup(hash)
		if err == nil {
			return key, true, nil
		}
		if errors.Is(err, store.ErrNotFound) {
			return models.AdminKey{}, false, nil
		}
		return models.AdminKey{}, false, err
	}

	if key, matched, err := tryLookup(candidates.Current); err != nil {
		return models.AdminKey{}, err
	} else if matched {
		return key, nil
	}

	for _, previousHash := range candidates.Previous {
		key, matched, err := tryLookup(previousHash)
		if err != nil {
			return models.AdminKey{}, err
		}
		if matched {
			return key, nil
		}
	}

	key, matched, err := tryLookup(candidates.Legacy)
	if err != nil {
		return models.AdminKey{}, err
	}
	if !matched {
		return models.AdminKey{}, ErrUnauthorized
	}

	if strings.TrimSpace(candidates.Current) == "" || candidates.Current == candidates.Legacy {
		return key, nil
	}
	upgrader, ok := st.(adminKeyHashUpgrader)
	if !ok {
		return key, nil
	}
	_ = upgrader.UpgradeAdminKeyHash(ctx, candidates.Legacy, candidates.Current)
	return key, nil
}

// HasScopeInList checks if the required scope is present in the scopes list,
// including backward-compatible umbrella scope checks.
func HasScopeInList(scopes []string, required string) bool {
	return hasScope(scopes, required)
}

func hasScope(scopes []string, required string) bool {
	if slices.Contains(scopes, required) {
		return true
	}

	// Backward compatibility during scope migration:
	// - admin:read remains an umbrella for all read-like granular scopes.
	// - admin:write remains an umbrella for all non-read granular scopes.
	if isReadLikeScope(required) {
		return slices.Contains(scopes, scopeAdminRead)
	}
	return slices.Contains(scopes, scopeAdminWrite)
}

func isReadLikeScope(required string) bool {
	if required == scopeAdminRead {
		return true
	}
	return strings.HasSuffix(required, ":read") ||
		strings.HasSuffix(required, ":export") ||
		strings.HasSuffix(required, ":simulate")
}
