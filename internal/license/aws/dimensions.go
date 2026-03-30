package aws

import (
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice/types"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// AWS Marketplace dimension names. These must match the dimensions defined in
// the AWS Marketplace product listing.
const (
	DimensionMaxTenants         = "max_tenants"
	DimensionMaxAgentsPerTenant = "max_agents_per_tenant"
	DimensionMaxActiveKeys      = "max_active_keys"
	DimensionMonthlyActions     = "monthly_actions"
	DimensionAuditRetentionDays = "audit_retention_days"
	DimensionApprovalWorkflows  = "approval_workflows"
	DimensionEvidenceExport     = "evidence_export"
	DimensionIntegrations       = "integrations"
	DimensionCustomPolicies     = "custom_policies"
)

// mapDimensionsToEntitlements converts AWS Marketplace entitlement dimensions
// into the internal Entitlements struct. Starts from PaidTierDefaults and
// overrides with any dimension values present in the entitlement response.
// Unknown dimensions are logged and ignored.
func mapDimensionsToEntitlements(awsEntitlements []types.Entitlement) license.Entitlements {
	ent := license.PaidTierDefaults()

	for _, e := range awsEntitlements {
		if e.Dimension == nil || e.Value == nil {
			continue
		}

		dim := *e.Dimension
		switch dim {
		case DimensionMaxTenants:
			if v := intValue(e.Value); v > 0 {
				ent.MaxTenants = v
			}
		case DimensionMaxAgentsPerTenant:
			if v := intValue(e.Value); v > 0 {
				ent.MaxAgentsPerTenant = v
			}
		case DimensionMaxActiveKeys:
			if v := intValue(e.Value); v > 0 {
				ent.MaxActiveKeys = v
			}
		case DimensionMonthlyActions:
			if v := intValue(e.Value); v > 0 {
				ent.MonthlyActionLimit = int64(v)
			}
		case DimensionAuditRetentionDays:
			if v := intValue(e.Value); v > 0 {
				ent.AuditRetentionDays = v
			}
		case DimensionApprovalWorkflows:
			ent.ApprovalWorkflows = boolValue(e.Value)
		case DimensionEvidenceExport:
			ent.EvidenceExport = boolValue(e.Value)
		case DimensionIntegrations:
			ent.Integrations = boolValue(e.Value)
		case DimensionCustomPolicies:
			ent.CustomPolicies = boolValue(e.Value)
		default:
			slog.Warn("unknown AWS Marketplace dimension", "dimension", dim)
		}
	}

	return ent
}

// intValue extracts an integer from an EntitlementValue, preferring IntegerValue
// then falling back to DoubleValue (truncated).
func intValue(v *types.EntitlementValue) int {
	if v.IntegerValue != nil {
		return int(*v.IntegerValue)
	}
	if v.DoubleValue != nil {
		return int(*v.DoubleValue)
	}
	return 0
}

// boolValue extracts a boolean from an EntitlementValue, defaulting to false.
func boolValue(v *types.EntitlementValue) bool {
	if v.BooleanValue != nil {
		return *v.BooleanValue
	}
	return false
}
