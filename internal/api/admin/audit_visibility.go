package admin

import (
	"context"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// auditVisibilityFloor returns the earliest timestamp that audit events should
// be visible for the current license tier. Free-tier installations are limited
// to the last N days (defined by AuditRetentionDays in entitlements). Paid-tier
// (or when no validator is configured) returns nil, meaning no restriction.
//
// This implements "soft retention": data is never deleted based on the tier,
// only hidden from query results. Upgrading to paid tier restores full
// visibility without data loss.
func (d *Dependencies) auditVisibilityFloor() *time.Time {
	if d.LicenseValidator == nil {
		return nil
	}

	ent := d.LicenseValidator.Entitlements()
	if ent.Tier != "free" {
		return nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -ent.AuditRetentionDays)
	return &cutoff
}

// applyVisibilityFloor returns the more restrictive of the user-supplied 'from'
// parameter and the tier-based visibility floor. If the user explicitly
// requested a narrower window (i.e. 'from' is after the floor), their value is
// preserved. If 'from' is nil or earlier than the floor, the floor is applied.
func applyVisibilityFloor(from, floor *time.Time) *time.Time {
	if floor == nil {
		return from
	}
	if from == nil {
		return floor
	}
	if from.Before(*floor) {
		return floor
	}
	return from
}

// getEffectiveAuditRetentionDays returns the effective audit retention days
// value, which is the configured setting value capped by the license tier's
// maximum allowed retention. For paid/trial tiers with unlimited retention
// (-1), the setting value is used as-is. For free tier, the setting is capped
// at the license maximum (typically 7 days).
func (d *Dependencies) getEffectiveAuditRetentionDays(ctx context.Context, ent license.Entitlements) int {
	// Get configured retention from settings, default to DefaultAuditRetentionDays
	settingValue := license.DefaultAuditRetentionDays
	if d.Store != nil {
		if val, err := d.Store.GetAuditRetentionDays(ctx); err == nil && val > 0 {
			settingValue = val
		}
	}

	// If license allows unlimited retention, return the setting value
	if license.IsUnlimited(ent.AuditRetentionDays) {
		return settingValue
	}

	// Otherwise, cap the setting value by the license maximum
	if settingValue > ent.AuditRetentionDays {
		return ent.AuditRetentionDays
	}

	return settingValue
}
