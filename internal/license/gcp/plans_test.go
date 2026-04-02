package gcp

import (
	"encoding/json"
	"testing"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func TestMapPlanToEntitlements_Starter(t *testing.T) {
	ent := mapPlanToEntitlements(PlanStarter, nil)

	if ent.MaxTenants != 5 {
		t.Errorf("MaxTenants = %d, want 5", ent.MaxTenants)
	}
	if ent.MaxAgentsPerTenant != 10 {
		t.Errorf("MaxAgentsPerTenant = %d, want 10", ent.MaxAgentsPerTenant)
	}
	if ent.MaxActiveKeys != 20 {
		t.Errorf("MaxActiveKeys = %d, want 20", ent.MaxActiveKeys)
	}
	if ent.MonthlyActionLimit != 10_000 {
		t.Errorf("MonthlyActionLimit = %d, want 10000", ent.MonthlyActionLimit)
	}
	if ent.AuditRetentionDays != 30 {
		t.Errorf("AuditRetentionDays = %d, want 30", ent.AuditRetentionDays)
	}
	if ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be false for starter")
	}
	if ent.EvidenceExport {
		t.Error("EvidenceExport should be false for starter")
	}
	if ent.CustomPolicies {
		t.Error("CustomPolicies should be false for starter")
	}
}

func TestMapPlanToEntitlements_Pro(t *testing.T) {
	ent := mapPlanToEntitlements(PlanPro, nil)

	if ent.MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25", ent.MaxTenants)
	}
	if ent.MaxAgentsPerTenant != 50 {
		t.Errorf("MaxAgentsPerTenant = %d, want 50", ent.MaxAgentsPerTenant)
	}
	if ent.MonthlyActionLimit != 100_000 {
		t.Errorf("MonthlyActionLimit = %d, want 100000", ent.MonthlyActionLimit)
	}
	if ent.AuditRetentionDays != 90 {
		t.Errorf("AuditRetentionDays = %d, want 90", ent.AuditRetentionDays)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true for pro")
	}
	if !ent.EvidenceExport {
		t.Error("EvidenceExport should be true for pro")
	}
	if ent.CustomPolicies {
		t.Error("CustomPolicies should be false for pro")
	}
}

func TestMapPlanToEntitlements_Enterprise(t *testing.T) {
	ent := mapPlanToEntitlements(PlanEnterprise, nil)

	// Enterprise should use PaidTierDefaults (unlimited = -1).
	if !license.IsUnlimited(ent.MaxTenants) {
		t.Errorf("Enterprise MaxTenants should be unlimited, got %d", ent.MaxTenants)
	}
}

func TestMapPlanToEntitlements_UnknownPlan(t *testing.T) {
	// Unknown plans should fall back to paid defaults (unlimited = -1).
	ent := mapPlanToEntitlements("custom-plan-xyz", nil)
	if !license.IsUnlimited(ent.MaxTenants) {
		t.Errorf("Unknown plan should produce unlimited MaxTenants, got %d", ent.MaxTenants)
	}
}

func TestMapPlanToEntitlements_CustomProperties(t *testing.T) {
	props := entitlementProperties{
		MaxTenants:         99,
		MaxAgentsPerTenant: 200,
		ApprovalWorkflows:  true,
		EvidenceExport:     true,
	}
	raw, _ := json.Marshal(props)

	ent := mapPlanToEntitlements(PlanStarter, raw)

	if ent.MaxTenants != 99 {
		t.Errorf("MaxTenants = %d, want 99 (custom override)", ent.MaxTenants)
	}
	if ent.MaxAgentsPerTenant != 200 {
		t.Errorf("MaxAgentsPerTenant = %d, want 200 (custom override)", ent.MaxAgentsPerTenant)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true (custom override)")
	}
}

func TestMapPlanToEntitlements_InvalidProperties(t *testing.T) {
	// Invalid JSON should be ignored, not panic.
	ent := mapPlanToEntitlements(PlanPro, json.RawMessage(`{invalid`))
	if ent.MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (should keep plan defaults on invalid properties)", ent.MaxTenants)
	}
}
