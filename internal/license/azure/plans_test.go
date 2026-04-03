package azure

import (
	"testing"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func TestMapPlanToEntitlements_Starter(t *testing.T) {
	ent := mapPlanToEntitlements(PlanStarter)

	if ent.MaxTenants != 5 {
		t.Errorf("MaxTenants = %d, want 5", ent.MaxTenants)
	}
	if ent.MaxAgentsPerTenant != 10 {
		t.Errorf("MaxAgentsPerTenant = %d, want 10", ent.MaxAgentsPerTenant)
	}
	if ent.MonthlyActionLimit != 10_000 {
		t.Errorf("MonthlyActionLimit = %d, want 10000", ent.MonthlyActionLimit)
	}
	if ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be false for starter")
	}
	if ent.EvidenceExport {
		t.Error("EvidenceExport should be false for starter")
	}
}

func TestMapPlanToEntitlements_Pro(t *testing.T) {
	ent := mapPlanToEntitlements(PlanPro)

	if ent.MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25", ent.MaxTenants)
	}
	if ent.MonthlyActionLimit != 100_000 {
		t.Errorf("MonthlyActionLimit = %d, want 100000", ent.MonthlyActionLimit)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true for pro")
	}
}

func TestMapPlanToEntitlements_Enterprise(t *testing.T) {
	ent := mapPlanToEntitlements(PlanEnterprise)

	if !license.IsUnlimited(ent.MaxTenants) {
		t.Errorf("Enterprise MaxTenants should be unlimited, got %d", ent.MaxTenants)
	}
}

func TestMapPlanToEntitlements_Unknown(t *testing.T) {
	ent := mapPlanToEntitlements("custom-plan")

	if !license.IsUnlimited(ent.MaxTenants) {
		t.Errorf("Unknown plan should use paid defaults (unlimited), got %d", ent.MaxTenants)
	}
}

func TestIsActiveStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{StatusSubscribed, true},
		{StatusPendingFulfillmentStart, false},
		{StatusSuspended, false},
		{StatusUnsubscribed, false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := isActiveStatus(tt.status); got != tt.want {
				t.Errorf("isActiveStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
