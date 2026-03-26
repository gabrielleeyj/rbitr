package license

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFreeTierDefaults(t *testing.T) {
	e := FreeTierDefaults()
	assert.Equal(t, "free", e.Tier)
	assert.Equal(t, 1, e.MaxTenants)
	assert.Equal(t, 1, e.MaxAgentsPerTenant)
	assert.Equal(t, 1, e.MaxActiveKeys)
	assert.Equal(t, int64(10_000), e.MonthlyActionLimit)
	assert.Equal(t, 7, e.AuditRetentionDays)
	assert.False(t, e.ApprovalWorkflows)
	assert.False(t, e.EvidenceExport)
	assert.False(t, e.Integrations)
	assert.True(t, e.CustomPolicies)
}

func TestPaidTierDefaults(t *testing.T) {
	e := PaidTierDefaults()
	assert.Equal(t, "paid", e.Tier)
	assert.Equal(t, Unlimited, e.MaxTenants)
	assert.Equal(t, Unlimited, e.MaxAgentsPerTenant)
	assert.Equal(t, Unlimited, e.MaxActiveKeys)
	assert.True(t, IsUnlimited64(e.MonthlyActionLimit))
	assert.Equal(t, Unlimited, e.AuditRetentionDays) // Paid tier has unlimited retention
	assert.True(t, e.ApprovalWorkflows)
	assert.True(t, e.EvidenceExport)
	assert.True(t, e.Integrations)
	assert.True(t, e.CustomPolicies)
}

func TestDefaultsForTier(t *testing.T) {
	assert.Equal(t, "free", DefaultsForTier("free").Tier)
	assert.Equal(t, "paid", DefaultsForTier("paid").Tier)
	assert.Equal(t, "free", DefaultsForTier("unknown").Tier)
}

func TestHasFeature(t *testing.T) {
	paid := PaidTierDefaults()
	free := FreeTierDefaults()

	tests := []struct {
		feature string
		paid    bool
		free    bool
	}{
		{"approval_workflows", true, false},
		{"evidence_export", true, false},
		{"integrations", true, false},
		{"custom_policies", true, true},
		{"nonexistent", false, false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.paid, paid.HasFeature(tt.feature), "paid: %s", tt.feature)
		assert.Equal(t, tt.free, free.HasFeature(tt.feature), "free: %s", tt.feature)
	}
}

func TestIsUnlimited(t *testing.T) {
	assert.True(t, IsUnlimited(Unlimited))
	assert.False(t, IsUnlimited(0))
	assert.False(t, IsUnlimited(1))

	assert.True(t, IsUnlimited64(int64(Unlimited)))
	assert.False(t, IsUnlimited64(0))
	assert.False(t, IsUnlimited64(10_000))
}

func TestMergeOverDefaults_NilClaims(t *testing.T) {
	result := MergeOverDefaults("paid", nil)
	assert.Equal(t, PaidTierDefaults(), result)
}

func TestMergeOverDefaults_PartialOverride(t *testing.T) {
	claims := &Entitlements{
		AuditRetentionDays: 365,
		ApprovalWorkflows:  true,
		EvidenceExport:     true,
		Integrations:       true,
		CustomPolicies:     true,
	}

	result := MergeOverDefaults("paid", claims)
	assert.Equal(t, 365, result.AuditRetentionDays)
	// Other numeric fields should keep paid defaults.
	assert.Equal(t, Unlimited, result.MaxTenants)
	assert.Equal(t, Unlimited, result.MaxAgentsPerTenant)
}

func TestMergeOverDefaults_FreeTierOverride(t *testing.T) {
	claims := &Entitlements{
		MaxTenants:        5,
		ApprovalWorkflows: false,
		EvidenceExport:    false,
		Integrations:      false,
		CustomPolicies:    false,
	}

	result := MergeOverDefaults("free", claims)
	assert.Equal(t, "free", result.Tier)
	assert.Equal(t, 5, result.MaxTenants)
	// Booleans: OR of default (false) and claims (false) = false.
	assert.False(t, result.ApprovalWorkflows)
}

func TestMergeOverDefaults_PaidBooleanCannotDowngrade(t *testing.T) {
	// A paid-tier key with boolean features set to false should NOT strip them.
	claims := &Entitlements{
		ApprovalWorkflows: false,
		EvidenceExport:    false,
		Integrations:      false,
		CustomPolicies:    false,
	}

	result := MergeOverDefaults("paid", claims)
	// Paid defaults are true; claims=false cannot downgrade them.
	assert.True(t, result.ApprovalWorkflows)
	assert.True(t, result.EvidenceExport)
	assert.True(t, result.Integrations)
	assert.True(t, result.CustomPolicies)
}
