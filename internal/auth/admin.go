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

const AdminKeyHeader = "X-Admin-Key"
const AuthorizationHeader = "Authorization"

func AdminKeyFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := bearerToken(r.Header.Get(AuthorizationHeader)); token != "" {
		return token
	}
	return r.Header.Get(AdminKeyHeader)
}

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

func AuthenticateAdmin(ctx context.Context, st store.StoreAPI, adminKey, requiredScope string) (models.AdminKey, error) {
	if adminKey == "" {
		return models.AdminKey{}, ErrUnauthorized
	}
	keyHash := utils.HashString(adminKey)
	key, err := st.GetAdminKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.AdminKey{}, ErrUnauthorized
		}
		return models.AdminKey{}, err
	}
	if !hasScope(key.Scopes, requiredScope) {
		return models.AdminKey{}, ErrForbidden
	}
	return key, nil
}

func hasScope(scopes []string, required string) bool {
	return slices.Contains(scopes, required)
}
