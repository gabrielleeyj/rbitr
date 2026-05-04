package admin

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// featureGate returns middleware that blocks requests when the named feature
// is not enabled by the current license entitlements.
func (d *Dependencies) featureGate(feature string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if d.LicenseProvider == nil {
				return next(c)
			}

			ent := d.LicenseProvider.Entitlements()
			if ent.HasFeature(feature) {
				return next(c)
			}

			return c.JSON(http.StatusForbidden, map[string]string{
				"error":      "FEATURE_NOT_AVAILABLE",
				"feature":    feature,
				fieldMessage: feature + " requires a paid or trial license. Upload a license key in Settings > License to unlock.",
			})
		}
	}
}

// handleEntitlements returns the current license entitlements for UI rendering.
func (d *Dependencies) handleEntitlements(c *echo.Context) error {
	if d.LicenseProvider == nil {
		return c.JSON(http.StatusOK, map[string]any{
			fieldTier: tierFreeStr,
			fieldFeatures: map[string]bool{
				featureApprovalWorkflows: false,
				featureEvidenceExport:    false,
				featureIntegrations:      false,
				featureCustomPolicies:    true,
			},
		})
	}

	ent := d.LicenseProvider.Entitlements()

	return c.JSON(http.StatusOK, map[string]any{
		fieldTier: ent.Tier,
		fieldFeatures: map[string]bool{
			featureApprovalWorkflows: ent.ApprovalWorkflows,
			featureEvidenceExport:    ent.EvidenceExport,
			featureIntegrations:      ent.Integrations,
			featureCustomPolicies:    ent.CustomPolicies,
		},
		"limits": map[string]any{
			"max_tenants":           ent.MaxTenants,
			"max_agents_per_tenant": ent.MaxAgentsPerTenant,
			"max_active_keys":       ent.MaxActiveKeys,
			"monthly_action_limit":  ent.MonthlyActionLimit,
			"audit_retention_days":  ent.AuditRetentionDays,
		},
	})
}
