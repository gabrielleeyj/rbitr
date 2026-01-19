package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

const AdminKeyHeader = "X-Admin-Key"

func AuthenticateAdmin(ctx context.Context, st store.StoreAPI, adminKey string, requiredScope string) (models.AdminKey, error) {
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
