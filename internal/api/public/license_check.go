package public

import (
	"context"
)

// checkFeatureAccess returns an error message if the feature is not accessible.
// Returns empty string if access is allowed.
func (d *Dependencies) checkFeatureAccess(_ context.Context, _ string, feature string) string {
	ent := d.LicenseValidator.Entitlements()

	if ent.HasFeature(feature) {
		return ""
	}

	// Feature not accessible - provide upgrade message based on tier
	if ent.Tier == "trial" {
		return "Your trial license has expired. Upload a paid license key to continue using premium features."
	}

	return "This feature is not available on the free tier. Upload a trial or paid license key to unlock."
}
