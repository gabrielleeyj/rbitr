package admin

import (
	"context"
	"fmt"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// provisioningViolation describes a limit that would be exceeded.
type provisioningViolation struct {
	Resource string `json:"resource"`
	Current  int    `json:"current"`
	Limit    int    `json:"limit"`
	Message  string `json:"message"`
}

// checkTenantLimit verifies the tenant count is below the entitlement limit.
// Returns nil if the limit is not exceeded or the license validator is nil.
//
//nolint:nilnil // nil means allowed.
func (d *Dependencies) checkTenantLimit(ctx context.Context) (*provisioningViolation, error) {
	if d.LicenseValidator == nil {
		return nil, nil
	}

	ent := d.LicenseValidator.Entitlements()
	if license.IsUnlimited(ent.MaxTenants) {
		return nil, nil
	}

	count, err := d.Store.CountTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("count tenants: %w", err)
	}

	if count >= ent.MaxTenants {
		return &provisioningViolation{
			Resource: "tenants",
			Current:  count,
			Limit:    ent.MaxTenants,
			Message:  "Tenant limit reached. Upload a license key to create more tenants.",
		}, nil
	}

	return nil, nil
}

// checkActiveKeyLimit verifies the active key count for a tenant is below the entitlement limit.
// Returns nil if the limit is not exceeded or the license validator is nil.
//
//nolint:nilnil // nil means allowed.
func (d *Dependencies) checkActiveKeyLimit(ctx context.Context, tenantID string) (*provisioningViolation, error) {
	if d.LicenseValidator == nil {
		return nil, nil
	}

	ent := d.LicenseValidator.Entitlements()
	if license.IsUnlimited(ent.MaxActiveKeys) {
		return nil, nil
	}

	count, err := d.Store.CountActiveKeysByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("count active keys: %w", err)
	}

	if count >= ent.MaxActiveKeys {
		return &provisioningViolation{
			Resource: "active_keys",
			Current:  count,
			Limit:    ent.MaxActiveKeys,
			Message:  "Active key limit reached. Revoke an existing key or upload a license key.",
		}, nil
	}

	return nil, nil
}
