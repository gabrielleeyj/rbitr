package store

import (
	"context"
	"fmt"
)

// CountTenants returns the total number of non-deleted tenants.
func (s *Store) CountTenants(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM rbitr.tenants WHERE deleted_at IS NULL`

	var count int
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count tenants: %w", err)
	}
	return count, nil
}

// CountActiveKeysByTenant returns the number of non-revoked keys for a tenant.
func (s *Store) CountActiveKeysByTenant(ctx context.Context, tenantID string) (int, error) {
	query := `SELECT COUNT(*) FROM rbitr.tenant_keys WHERE tenant_id = $1 AND revoked_at IS NULL`

	var count int
	if err := s.db.QueryRowContext(ctx, query, tenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count active keys: %w", err)
	}
	return count, nil
}
