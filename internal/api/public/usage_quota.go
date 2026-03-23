package public

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// usageQuotaViolation is returned when a tenant exceeds their monthly action limit.
type usageQuotaViolation struct {
	Limit   int64  `json:"limit"`
	Used    int64  `json:"used"`
	Period  string `json:"period"`
	Message string `json:"message"`
}

// currentPeriod returns the current billing period in YYYY-MM format.
func currentPeriod() string {
	return time.Now().UTC().Format("2006-01")
}

// enforceUsageQuota checks whether the tenant has exceeded their monthly action
// limit. If the license is unlimited, metering is skipped entirely. Otherwise,
// the counter is atomically incremented and checked against the limit.
//
//nolint:nilnil // nil violation with nil error means request is allowed.
func (d *Dependencies) enforceUsageQuota(ctx context.Context, tenantID string) (*usageQuotaViolation, error) {
	if d.LicenseValidator == nil {
		return nil, nil
	}

	ent := d.LicenseValidator.Entitlements()

	// Paid tier: unlimited actions, skip metering entirely.
	if license.IsUnlimited64(ent.MonthlyActionLimit) {
		return nil, nil
	}

	if d.Store == nil {
		slog.Warn("usage quota check skipped: store is nil", "tenant_id", tenantID)
		return nil, nil
	}

	period := currentPeriod()
	count, err := d.Store.IncrementUsageMeter(ctx, tenantID, period)
	if err != nil {
		return nil, fmt.Errorf("increment usage meter: %w", err)
	}

	if count > ent.MonthlyActionLimit {
		return &usageQuotaViolation{
			Limit:   ent.MonthlyActionLimit,
			Used:    count,
			Period:  period,
			Message: "Monthly action limit exceeded. Upload a license key to remove limits.",
		}, nil
	}

	return nil, nil
}

// secondsUntilPeriodReset returns the number of seconds until the first of the
// next UTC month. Used for Retry-After headers on quota violation responses.
func secondsUntilPeriodReset() int {
	now := time.Now().UTC()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return int(time.Until(nextMonth).Seconds()) + 1
}
