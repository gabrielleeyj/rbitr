package license

import "time"

// Unlimited indicates no limit for a numeric entitlement.
const Unlimited = -1

// Entitlements describes what the current installation is allowed to do.
type Entitlements struct {
	Tier               string `json:"tier"`
	MaxTenants         int    `json:"max_tenants"`
	MaxAgentsPerTenant int    `json:"max_agents_per_tenant"`
	MaxActiveKeys      int    `json:"max_active_keys"`
	MonthlyActionLimit int64  `json:"monthly_action_limit"`
	AuditRetentionDays int    `json:"audit_retention_days"`
	ApprovalWorkflows  bool   `json:"approval_workflows"`
	EvidenceExport     bool   `json:"evidence_export"`
	Integrations       bool   `json:"integrations"`
	CustomPolicies     bool   `json:"custom_policies"`
}

// LicenseInfo contains the validated license metadata and resolved entitlements.
type LicenseInfo struct {
	Valid        bool         `json:"valid"`
	Tier         string       `json:"tier"`
	KeyVersion   int          `json:"key_version"`
	Licensee     string       `json:"licensee"`
	Email        string       `json:"email"`
	IssuedAt     time.Time    `json:"issued_at"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Entitlements Entitlements `json:"entitlements"`
}

// FreeTierDefaults returns the entitlements for an installation with no license key.
func FreeTierDefaults() Entitlements {
	return Entitlements{
		Tier:               "free",
		MaxTenants:         1,
		MaxAgentsPerTenant: 1,
		MaxActiveKeys:      1,
		MonthlyActionLimit: 10_000,
		AuditRetentionDays: 7,
		ApprovalWorkflows:  false,
		EvidenceExport:     false,
		Integrations:       false,
		CustomPolicies:     false,
	}
}

// PaidTierDefaults returns the entitlements for a valid paid license key.
func PaidTierDefaults() Entitlements {
	return Entitlements{
		Tier:               "paid",
		MaxTenants:         Unlimited,
		MaxAgentsPerTenant: Unlimited,
		MaxActiveKeys:      Unlimited,
		MonthlyActionLimit: int64(Unlimited),
		AuditRetentionDays: 90,
		ApprovalWorkflows:  true,
		EvidenceExport:     true,
		Integrations:       true,
		CustomPolicies:     true,
	}
}

// DefaultsForTier returns the default entitlements for the given tier string.
func DefaultsForTier(tier string) Entitlements {
	if tier == "paid" {
		return PaidTierDefaults()
	}
	return FreeTierDefaults()
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
