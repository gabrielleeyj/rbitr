package admin

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// featureGate returns middleware that blocks requests when the named feature
// is not enabled by the current license entitlements. Read-only GET requests
// are allowed through so operators can still view configuration; only mutating
// operations are gated.
func (d *Dependencies) featureGate(feature string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if d.LicenseValidator == nil {
				return next(c)
			}

			ent := d.LicenseValidator.Entitlements()
			if ent.HasFeature(feature) {
				return next(c)
			}

			return c.JSON(http.StatusForbidden, map[string]string{
				"error":   "FEATURE_NOT_AVAILABLE",
				"feature": feature,
				"message": feature + " requires a paid license. Upload a license key in Settings > License to unlock.",
			})
		}
	}
}

// handleEntitlements returns the current license entitlements for UI rendering.
func (d *Dependencies) handleEntitlements(c *echo.Context) error {
	if d.LicenseValidator == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"tier": "free",
			"features": map[string]bool{
				"approval_workflows": false,
				"evidence_export":    false,
				"integrations":       false,
				"custom_policies":    false,
			},
		})
	}

	ent := d.LicenseValidator.Entitlements()
	return c.JSON(http.StatusOK, map[string]any{
		"tier": ent.Tier,
		"features": map[string]bool{
			"approval_workflows": ent.ApprovalWorkflows,
			"evidence_export":    ent.EvidenceExport,
			"integrations":       ent.Integrations,
			"custom_policies":    ent.CustomPolicies,
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
