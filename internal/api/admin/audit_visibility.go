package admin

import "time"

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
