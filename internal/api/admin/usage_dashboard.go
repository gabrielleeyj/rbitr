package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	defaultUsageHistoryMonths = 6
	maxUsageHistoryMonths     = 24
	percentMultiplier         = 100.0
)

// usageGauge represents a single resource usage metric with its limit.
type usageGauge struct {
	Used    int64   `json:"used"`
	Limit   int64   `json:"limit"`
	Percent float64 `json:"pct"`
}

// handleUsageSummary returns the current period usage summary across all
// resources: governed actions, tenants, agents, and active keys.
func (d *Dependencies) handleUsageSummary(c *echo.Context) error {
	ctx := c.Request().Context()

	ent := license.FreeTierDefaults()
	var licenseResp map[string]any
	if d.LicenseValidator != nil {
		ent = d.LicenseValidator.Entitlements()
		info := d.LicenseValidator.Info()
		licenseResp = map[string]any{
			"valid":      info.Valid,
			"tier":       info.Tier,
			"upload_url": "/settings/license",
		}
	} else {
		licenseResp = map[string]any{
			"valid":      false,
			"tier":       "free",
			"upload_url": "/settings/license",
		}
	}

	period := currentPeriod()

	// Get governed actions usage for current period.
	var actionsUsed int64
	if d.Store != nil {
		total, err := d.Store.GetTotalUsageForPeriod(ctx, period)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to load usage data",
			})
		}
		actionsUsed = total
	}

	// Get tenant count.
	var tenantCount int
	if d.Store != nil {
		count, err := d.Store.CountTenants(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to count tenants",
			})
		}
		tenantCount = count
	}

	// Get effective audit retention days (from settings, capped by license)
	effectiveRetention := d.getEffectiveAuditRetentionDays(ctx, ent)

	return c.JSON(http.StatusOK, map[string]any{
		"tier":   ent.Tier,
		"period": period,
		"usage": map[string]any{
			"governed_actions": buildGauge(actionsUsed, ent.MonthlyActionLimit),
			"tenants":          buildGauge(int64(tenantCount), int64(ent.MaxTenants)),
		},
		"features": map[string]bool{
			"approval_workflows": ent.ApprovalWorkflows,
			"evidence_export":    ent.EvidenceExport,
			"integrations":       ent.Integrations,
			"custom_policies":    ent.CustomPolicies,
		},
		"audit_retention_days":     effectiveRetention,
		"audit_retention_days_max": ent.AuditRetentionDays,
		"license":                  licenseResp,
	})
}

// handleUsageHistory returns historical usage data aggregated across all
// tenants for the last N months (default 6, max 24).
func (d *Dependencies) handleUsageHistory(c *echo.Context) error {
	ctx := c.Request().Context()

	months := defaultUsageHistoryMonths
	if qp := c.QueryParam("months"); qp != "" {
		if parsed, err := strconv.Atoi(qp); err == nil && parsed > 0 {
			months = parsed
		}
	}
	if months > maxUsageHistoryMonths {
		months = maxUsageHistoryMonths
	}

	if d.Store == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"periods": []any{},
		})
	}

	records, err := d.Store.ListAggregatedUsageHistory(ctx, months)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to load usage history",
		})
	}

	// Determine the action limit for percentage calculations.
	var actionLimit int64
	if d.LicenseValidator != nil {
		actionLimit = d.LicenseValidator.Entitlements().MonthlyActionLimit
	} else {
		actionLimit = license.FreeTierDefaults().MonthlyActionLimit
	}

	periods := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		periods = append(periods, map[string]any{
			"period":       rec.Period,
			"action_count": rec.ActionCount,
			"pct":          calcPercent(rec.ActionCount, actionLimit),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"periods": periods,
	})
}

// currentPeriod returns the current billing period in YYYY-MM format.
func currentPeriod() string {
	return time.Now().UTC().Format("2006-01")
}

// buildGauge constructs a usage gauge with percentage. Unlimited limits (-1)
// result in 0% utilization.
func buildGauge(used, limit int64) usageGauge {
	return usageGauge{
		Used:    used,
		Limit:   limit,
		Percent: calcPercent(used, limit),
	}
}

// calcPercent computes the percentage of used/limit. Returns 0 for unlimited
// limits (negative values).
func calcPercent(used, limit int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(used) / float64(limit) * percentMultiplier
}
