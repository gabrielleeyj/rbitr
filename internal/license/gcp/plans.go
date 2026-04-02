package gcp

import (
	"encoding/json"
	"log/slog"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

// GCP Marketplace plan identifiers. These must match the plans defined in the
// GCP Marketplace product listing.
const (
	PlanStarter    = "starter"
	PlanPro        = "pro"
	PlanEnterprise = "enterprise"
)

// entitlementProperties holds the custom properties that may be present in
// a GCP Marketplace entitlement's InputProperties field.
type entitlementProperties struct {
	MaxTenants         int  `json:"max_tenants"`
	MaxAgentsPerTenant int  `json:"max_agents_per_tenant"`
	MaxActiveKeys      int  `json:"max_active_keys"`
	MonthlyActions     int  `json:"monthly_actions"`
	AuditRetentionDays int  `json:"audit_retention_days"`
	ApprovalWorkflows  bool `json:"approval_workflows"`
	EvidenceExport     bool `json:"evidence_export"`
	Integrations       bool `json:"integrations"`
	CustomPolicies     bool `json:"custom_policies"`
}

// mapPlanToEntitlements converts a GCP plan name and optional properties
// into the internal Entitlements struct. Starts from PaidTierDefaults and
// overrides with plan-specific values or custom properties.
func mapPlanToEntitlements(plan string, inputProperties json.RawMessage) license.Entitlements {
	ent := license.PaidTierDefaults()

	// Apply plan-specific overrides.
	switch plan {
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
		slog.Warn("unknown GCP Marketplace plan, using paid defaults", "plan", plan)
	}

	// If custom properties are present, override plan defaults.
	if len(inputProperties) > 0 {
		applyCustomProperties(&ent, inputProperties)
	}

	return ent
}

// applyCustomProperties overrides entitlements with any non-zero custom
// property values from the GCP entitlement's InputProperties.
func applyCustomProperties(ent *license.Entitlements, raw json.RawMessage) {
	var props entitlementProperties
	if err := json.Unmarshal(raw, &props); err != nil {
		slog.Warn("failed to parse GCP entitlement properties", "error", err)
		return
	}

	if props.MaxTenants > 0 {
		ent.MaxTenants = props.MaxTenants
	}
	if props.MaxAgentsPerTenant > 0 {
		ent.MaxAgentsPerTenant = props.MaxAgentsPerTenant
	}
	if props.MaxActiveKeys > 0 {
		ent.MaxActiveKeys = props.MaxActiveKeys
	}
	if props.MonthlyActions > 0 {
		ent.MonthlyActionLimit = int64(props.MonthlyActions)
	}
	if props.AuditRetentionDays > 0 {
		ent.AuditRetentionDays = props.AuditRetentionDays
	}
	// Boolean fields always override when custom properties are present.
	ent.ApprovalWorkflows = props.ApprovalWorkflows
	ent.EvidenceExport = props.EvidenceExport
	ent.Integrations = props.Integrations
	ent.CustomPolicies = props.CustomPolicies
}
