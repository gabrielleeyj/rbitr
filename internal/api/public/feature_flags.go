package public

import (
	"context"
	"errors"

	"github.com/gabrielleeyj/rbitr/internal/store"
)

func (d *Dependencies) disableXTenantKeyEnabled(ctx context.Context) bool {
	if d.Store == nil {
		return d.Config.DisableXTenantKey
	}
	enabled, err := d.Store.GetDisableXTenantKey(ctx)
	if err == nil {
		return enabled
	}
	if errors.Is(err, store.ErrNotFound) {
		return d.Config.DisableXTenantKey
	}
	return d.Config.DisableXTenantKey
}

func (d *Dependencies) featureRateLimitingEnabled(ctx context.Context) bool {
	if d.Store == nil {
		return d.Config.FeatureRateLimiting
	}
	enabled, err := d.Store.GetFeatureRateLimiting(ctx)
	if err == nil {
		return enabled
	}
	if errors.Is(err, store.ErrNotFound) {
		return d.Config.FeatureRateLimiting
	}
	return d.Config.FeatureRateLimiting
}

func (d *Dependencies) featureArgConstraintsEnabled(ctx context.Context) bool {
	if d.Store == nil {
		return d.Config.FeatureArgConstraints
	}
	enabled, err := d.Store.GetFeatureArgConstraints(ctx)
	if err == nil {
		return enabled
	}
	if errors.Is(err, store.ErrNotFound) {
		return d.Config.FeatureArgConstraints
	}
	return d.Config.FeatureArgConstraints
}

func (d *Dependencies) featureFileGovernanceEnabled(_ context.Context) bool {
	return d.Config.FeatureFileGovernance
}
