package azure

import (
	"log/slog"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// Azure Marketplace plan identifiers. These must match the plans defined in
// the Azure Marketplace SaaS offer.
const (
	PlanStarter    = "starter"
	PlanPro        = "pro"
	PlanEnterprise = "enterprise"
)

// Subscription status values from the SaaS Fulfillment API v2.
const (
	StatusPendingFulfillmentStart = "PendingFulfillmentStart"
	StatusSubscribed              = "Subscribed"
	StatusSuspended               = "Suspended"
	StatusUnsubscribed            = "Unsubscribed"
)

// mapPlanToEntitlements converts an Azure plan ID into the internal
// Entitlements struct. Follows the same plan tiers as GCP.
func mapPlanToEntitlements(planID string) license.Entitlements {
	ent := license.PaidTierDefaults()

	switch planID {
	case PlanStarter:
		ent.MaxTenants = 5
		ent.MaxAgentsPerTenant = 10
		ent.MaxActiveKeys = 20
		ent.MonthlyActionLimit = 10_000
		ent.AuditRetentionDays = 30
		ent.ApprovalWorkflows = false
		ent.EvidenceExport = false
		ent.CustomPolicies = false
	case PlanPro:
		ent.MaxTenants = 25
		ent.MaxAgentsPerTenant = 50
		ent.MaxActiveKeys = 100
		ent.MonthlyActionLimit = 100_000
		ent.AuditRetentionDays = 90
		ent.ApprovalWorkflows = true
		ent.EvidenceExport = true
		ent.CustomPolicies = false
	case PlanEnterprise:
		// Enterprise uses PaidTierDefaults (unlimited/high limits).
	default:
		slog.Warn("unknown Azure Marketplace plan, using paid defaults", "plan", planID)
	}

	return ent
}

// isActiveStatus returns true if the subscription status allows usage.
func isActiveStatus(status string) bool {
	return status == StatusSubscribed
}
