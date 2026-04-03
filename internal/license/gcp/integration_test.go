package gcp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
	servicecontrol "google.golang.org/api/servicecontrol/v2"

	gcplicense "github.com/gabrielleeyj/rbitr/internal/license/gcp"
)

// stubProcurementClient implements gcp.ProcurementClient for integration tests.
type stubProcurementClient struct {
	entitlements []*procurement.Entitlement
	err          error
	approvedName string
}

func (s *stubProcurementClient) ListEntitlements(_ string) (*procurement.ListEntitlementsResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &procurement.ListEntitlementsResponse{
		Entitlements: s.entitlements,
	}, nil
}

func (s *stubProcurementClient) ApproveEntitlement(name string) error {
	s.approvedName = name
	return nil
}

// stubServiceControlClient implements gcp.ServiceControlClient for integration tests.
type stubServiceControlClient struct {
	reportCalls int
	lastReq     *servicecontrol.ReportRequest
	err         error
}

func (s *stubServiceControlClient) Report(_ string, req *servicecontrol.ReportRequest) error {
	if s.err != nil {
		return s.err
	}
	s.reportCalls++
	s.lastReq = req
	return nil
}

func TestGCPIntegration_FullLifecycle(t *testing.T) {
	// Phase 1: Create provider with a Pro plan entitlement.
	procClient := &stubProcurementClient{
		entitlements: []*procurement.Entitlement{
			{
				ProductExternalName: "rbitr-service",
				Plan:                gcplicense.PlanPro,
				State:               "ENTITLEMENT_ACTIVE",
				Account:             "accounts/acct-123",
			},
		},
	}

	provider, err := gcplicense.NewProvider(procClient, "my-provider", "rbitr-service")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	// Phase 2: Verify entitlements are loaded from Pro plan.
	ent := provider.Entitlements()
	if ent.MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25", ent.MaxTenants)
	}
	if ent.MonthlyActionLimit != 100_000 {
		t.Errorf("MonthlyActionLimit = %d, want 100000", ent.MonthlyActionLimit)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true for Pro plan")
	}

	info := provider.Info()
	if !info.Valid {
		t.Error("expected Valid = true")
	}
	if info.Tier != "paid" {
		t.Errorf("Tier = %q, want %q", info.Tier, "paid")
	}

	// Phase 3: Record usage and flush via reporter.
	scClient := &stubServiceControlClient{}
	reporter := gcplicense.NewReporter(scClient, "rbitr-service")

	if err := reporter.RecordUsage(context.Background(), "tenant-1", "2026-04", 5); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if err := reporter.RecordUsage(context.Background(), "tenant-1", "2026-04", 3); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	if reporter.PendingCount() != 2 {
		t.Errorf("PendingCount = %d, want 2", reporter.PendingCount())
	}

	if err := reporter.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if reporter.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0 after flush", reporter.PendingCount())
	}

	if scClient.reportCalls != 1 {
		t.Errorf("Report calls = %d, want 1", scClient.reportCalls)
	}

	// Phase 4: Start/stop background loops.
	ctx, cancel := context.WithCancel(context.Background())
	providerDone := make(chan struct{})
	reporterDone := make(chan struct{})

	go func() { provider.Start(ctx); close(providerDone) }()
	go func() { reporter.Start(ctx); close(reporterDone) }()

	cancel()

	select {
	case <-providerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("provider.Start did not return")
	}
	select {
	case <-reporterDone:
	case <-time.After(5 * time.Second):
		t.Fatal("reporter.Start did not return")
	}
}

func TestGCPIntegration_WebhookTriggeredRefresh(t *testing.T) {
	// Start with Starter plan.
	procClient := &stubProcurementClient{
		entitlements: []*procurement.Entitlement{
			{
				ProductExternalName: "rbitr-service",
				Plan:                gcplicense.PlanStarter,
				State:               "ENTITLEMENT_ACTIVE",
				Account:             "accounts/acct-123",
			},
		},
	}

	provider, err := gcplicense.NewProvider(procClient, "my-provider", "rbitr-service")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if provider.Entitlements().MaxTenants != 5 {
		t.Fatalf("initial MaxTenants = %d, want 5", provider.Entitlements().MaxTenants)
	}

	// Simulate plan upgrade via webhook — update the mock to return Pro.
	procClient.entitlements = []*procurement.Entitlement{
		{
			ProductExternalName: "rbitr-service",
			Plan:                gcplicense.PlanPro,
			State:               "ENTITLEMENT_ACTIVE",
			Account:             "accounts/acct-123",
		},
	}

	provider.UpdateEntitlement(context.Background(), "ent-123", "ENTITLEMENT_ACTIVE")

	if provider.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d after upgrade, want 25", provider.Entitlements().MaxTenants)
	}
}

func TestGCPIntegration_RefreshFailure_KeepsLastGood(t *testing.T) {
	procClient := &stubProcurementClient{
		entitlements: []*procurement.Entitlement{
			{
				ProductExternalName: "rbitr-service",
				Plan:                gcplicense.PlanStarter,
				State:               "ENTITLEMENT_ACTIVE",
				Account:             "accounts/acct-123",
			},
		},
	}

	provider, err := gcplicense.NewProvider(procClient, "my-provider", "rbitr-service")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if provider.Entitlements().MaxTenants != 5 {
		t.Fatalf("initial MaxTenants = %d, want 5", provider.Entitlements().MaxTenants)
	}

	// Simulate API failure — entitlements should be preserved.
	procClient.err = errors.New("throttled")

	provider.UpdateEntitlement(context.Background(), "ent-123", "ENTITLEMENT_ACTIVE")

	if provider.Entitlements().MaxTenants != 5 {
		t.Errorf("MaxTenants = %d after error, want 5 (last-known-good)", provider.Entitlements().MaxTenants)
	}
}
