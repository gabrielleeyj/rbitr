package license

import "time"

const (
	// Unlimited indicates no limit for a numeric entitlement.
	Unlimited = -1

	// freeMonthlyActionLimit is the default monthly action quota for free tier.
	freeMonthlyActionLimit = 10_000

	// FreeAuditRetentionMax is the maximum audit retention for free tier.
	FreeAuditRetentionMax = 7

	// PaidAuditRetentionMax is the maximum audit retention for paid tier.
	// Unlimited (-1) means no enforced maximum.
	PaidAuditRetentionMax = Unlimited

	// TrialDurationDays is the number of days for the free trial period.
	TrialDurationDays = 14

	// DefaultAuditRetentionDays is the default setting when none is configured.
	DefaultAuditRetentionDays = 365

	// Tier constants.
	tierFree  = "free"
	tierPaid  = "paid"
	tierTrial = "trial"

	hoursPerDay = 24
)

// Entitlements describes what the current installation is allowed to do.
type Entitlements struct {
	Tier               string `json:"tier"`
	MaxTenants         int    `json:"max_tenants"`
	MaxAgentsPerTenant int    `json:"max_agents_per_tenant"`
	MaxActiveKeys      int    `json:"max_active_keys"`
	MonthlyActionLimit int64  `json:"monthly_action_limit"`
	AuditRetentionDays int    `json:"audit_retention_days"` // Maximum allowed, not default
	ApprovalWorkflows  bool   `json:"approval_workflows"`
	EvidenceExport     bool   `json:"evidence_export"`
	Integrations       bool   `json:"integrations"`
	CustomPolicies     bool   `json:"custom_policies"`
}

// LicenseInfo contains the validated license metadata and resolved entitlements.
type LicenseInfo struct {
	Valid          bool         `json:"valid"`
	Tier           string       `json:"tier"`
	KeyVersion     int          `json:"key_version"`
	Licensee       string       `json:"licensee"`
	Email          string       `json:"email"`
	IssuedAt       time.Time    `json:"issued_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	TrialExpiresAt time.Time    `json:"trial_expires_at"` // For free tier, when trial ends
	Entitlements   Entitlements `json:"entitlements"`
}

// FreeTierDefaults returns the entitlements for an installation with no license key.
func FreeTierDefaults() Entitlements {
	return Entitlements{
		Tier:               tierFree,
		MaxTenants:         1,
		MaxAgentsPerTenant: 1,
		MaxActiveKeys:      1,
		MonthlyActionLimit: freeMonthlyActionLimit,
		AuditRetentionDays: FreeAuditRetentionMax,
		ApprovalWorkflows:  false,
		EvidenceExport:     false,
		Integrations:       false,
		CustomPolicies:     true,
	}
}

// PaidTierDefaults returns the entitlements for a valid paid license key.
func PaidTierDefaults() Entitlements {
	return Entitlements{
		Tier:               tierPaid,
		MaxTenants:         Unlimited,
		MaxAgentsPerTenant: Unlimited,
		MaxActiveKeys:      Unlimited,
		MonthlyActionLimit: int64(Unlimited),
		AuditRetentionDays: PaidAuditRetentionMax,
		ApprovalWorkflows:  true,
		EvidenceExport:     true,
		Integrations:       true,
		CustomPolicies:     true,
	}
}

// TrialTierDefaults returns the entitlements for a trial license key.
// Trial keys have same entitlements as paid tier but expire after 14 days.
func TrialTierDefaults() Entitlements {
	return Entitlements{
		Tier:               tierTrial,
		MaxTenants:         Unlimited,
		MaxAgentsPerTenant: Unlimited,
		MaxActiveKeys:      Unlimited,
		MonthlyActionLimit: int64(Unlimited),
		AuditRetentionDays: PaidAuditRetentionMax,
		ApprovalWorkflows:  true,
		EvidenceExport:     true,
		Integrations:       true,
		CustomPolicies:     true,
	}
}

// DefaultsForTier returns the default entitlements for the given tier string.
func DefaultsForTier(tier string) Entitlements {
	switch tier {
	case tierPaid:
		return PaidTierDefaults()
	case tierTrial:
		return TrialTierDefaults()
	default:
		return FreeTierDefaults()
	}
}

// HasFeature reports whether the named boolean feature is enabled.
func (e Entitlements) HasFeature(feature string) bool {
	switch feature {
	case "approval_workflows":
		return e.ApprovalWorkflows
	case "evidence_export":
		return e.EvidenceExport
	case "integrations":
		return e.Integrations
	case "custom_policies":
		return e.CustomPolicies
	default:
		return false
	}
}

// IsUnlimited reports whether the given numeric limit is unlimited.
func IsUnlimited(v int) bool {
	return v == Unlimited
}

// IsUnlimited64 reports whether the given int64 limit is unlimited.
func IsUnlimited64(v int64) bool {
	return v == int64(Unlimited)
}

// MergeOverDefaults merges non-zero claim values from claims on top of the tier
// defaults. This ensures that newer entitlement fields introduced in future key
// versions fall back to the tier default when an older key omits them.
func MergeOverDefaults(tier string, claims *Entitlements) Entitlements {
	defaults := DefaultsForTier(tier)
	if claims == nil {
		return defaults
	}

	merged := defaults
	merged.Tier = tier

	if claims.MaxTenants != 0 {
		merged.MaxTenants = claims.MaxTenants
	}
	if claims.MaxAgentsPerTenant != 0 {
		merged.MaxAgentsPerTenant = claims.MaxAgentsPerTenant
	}
	if claims.MaxActiveKeys != 0 {
		merged.MaxActiveKeys = claims.MaxActiveKeys
	}
	if claims.MonthlyActionLimit != 0 {
		merged.MonthlyActionLimit = claims.MonthlyActionLimit
	}
	if claims.AuditRetentionDays != 0 {
		merged.AuditRetentionDays = claims.AuditRetentionDays
	}

	// Boolean fields: claims can only upgrade from tier defaults, never
	// downgrade. A paid-tier key cannot strip features below paid defaults.
	// This prevents a legitimately-signed key with false values from silently
	// disabling paid features.
	merged.ApprovalWorkflows = defaults.ApprovalWorkflows || claims.ApprovalWorkflows
	merged.EvidenceExport = defaults.EvidenceExport || claims.EvidenceExport
	merged.Integrations = defaults.Integrations || claims.Integrations
	merged.CustomPolicies = defaults.CustomPolicies || claims.CustomPolicies

	return merged
}

// IsTrialActive returns true if the installation is in an active trial period.
// For free tier, this checks if TrialExpiresAt is in the future.
// For paid tier, always returns false (no trial needed).
func (info *LicenseInfo) IsTrialActive() bool {
	if info.Tier != tierFree {
		return false // Paid tier has no trial
	}
	if info.TrialExpiresAt.IsZero() {
		return false // No trial configured
	}
	return time.Now().Before(info.TrialExpiresAt)
}

// TrialDaysRemaining returns the number of days remaining in the trial.
// Returns 0 if trial is expired or not applicable.
func (info *LicenseInfo) TrialDaysRemaining() int {
	if !info.IsTrialActive() {
		return 0
	}
	duration := time.Until(info.TrialExpiresAt)
	days := int(duration.Hours() / hoursPerDay)
	if days < 0 {
		return 0
	}
	return days
}

// HasFeatureAccess returns true if the feature is accessible, considering both
// license entitlements and trial status.
func (info *LicenseInfo) HasFeatureAccess(feature string) bool {
	// Paid tier always has access to their entitled features
	if info.Tier == tierPaid {
		return info.Entitlements.HasFeature(feature)
	}

	// Free tier during active trial has access to all features
	if info.IsTrialActive() {
		return true
	}

	// Free tier after trial expires: only free tier features
	return info.Entitlements.HasFeature(feature)
}
