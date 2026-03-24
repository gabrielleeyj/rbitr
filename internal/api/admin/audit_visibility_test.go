package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func TestAuditVisibilityFloor_NilValidator(t *testing.T) {
	deps := &Dependencies{}
	floor := deps.auditVisibilityFloor()
	assert.Nil(t, floor, "nil validator should return nil floor (no restriction)")
}

func TestAuditVisibilityFloor_PaidTier(t *testing.T) {
	v := testLicenseValidator(t, "paid", license.Unlimited, license.Unlimited)
	deps := &Dependencies{LicenseValidator: v}

	floor := deps.auditVisibilityFloor()
	assert.Nil(t, floor, "paid tier should return nil floor (no restriction)")
}

func TestAuditVisibilityFloor_FreeTier(t *testing.T) {
	v := testLicenseValidator(t, "free", 1, 1)
	deps := &Dependencies{LicenseValidator: v}

	floor := deps.auditVisibilityFloor()
	assert.NotNil(t, floor, "free tier should return a visibility floor")

	expected := time.Now().UTC().AddDate(0, 0, -license.FreeTierDefaults().AuditRetentionDays)
	// Allow 2 seconds of tolerance for test execution time.
	assert.WithinDuration(t, expected, *floor, 2*time.Second)
}

func TestApplyVisibilityFloor_NilFloor(t *testing.T) {
	now := time.Now()
	result := applyVisibilityFloor(&now, nil)
	assert.Equal(t, &now, result, "nil floor should preserve original from")
}

func TestApplyVisibilityFloor_NilFrom(t *testing.T) {
	floor := time.Now().UTC().AddDate(0, 0, -7)
	result := applyVisibilityFloor(nil, &floor)
	assert.Equal(t, &floor, result, "nil from should use floor")
}

func TestApplyVisibilityFloor_FromBeforeFloor(t *testing.T) {
	floor := time.Now().UTC().AddDate(0, 0, -7)
	from := time.Now().UTC().AddDate(0, 0, -30) // 30 days ago, before the 7-day floor
	result := applyVisibilityFloor(&from, &floor)
	assert.Equal(t, &floor, result, "from before floor should be clamped to floor")
}

func TestApplyVisibilityFloor_FromAfterFloor(t *testing.T) {
	floor := time.Now().UTC().AddDate(0, 0, -7)
	from := time.Now().UTC().AddDate(0, 0, -3) // 3 days ago, after the 7-day floor
	result := applyVisibilityFloor(&from, &floor)
	assert.Equal(t, &from, result, "from after floor should be preserved")
}

func TestApplyVisibilityFloor_BothNil(t *testing.T) {
	result := applyVisibilityFloor(nil, nil)
	assert.Nil(t, result, "both nil should return nil")
}
