package azure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	azurelicense "github.com/gabrielleeyj/rbitr/internal/license/azure"
)

// stubFulfillmentClient implements azure.FulfillmentClient for integration tests.
type stubFulfillmentClient struct {
	subscription *azurelicense.Subscription
	resolved     *azurelicense.ResolvedSubscription
	resolveErr   error
	getSubErr    error
	activateErr  error
}

func (s *stubFulfillmentClient) ResolveToken(_ context.Context, _ string) (*azurelicense.ResolvedSubscription, error) {
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return s.resolved, nil
}

func (s *stubFulfillmentClient) GetSubscription(_ context.Context, _ string) (*azurelicense.Subscription, error) {
	if s.getSubErr != nil {
		return nil, s.getSubErr
	}
	return s.subscription, nil
}

func (s *stubFulfillmentClient) ActivateSubscription(_ context.Context, _, _ string) error {
	return s.activateErr
}

func (s *stubFulfillmentClient) ListSubscriptions(_ context.Context) ([]azurelicense.Subscription, error) {
	if s.subscription != nil {
		return []azurelicense.Subscription{*s.subscription}, nil
	}
	return nil, nil
}

// stubMeteringClient implements azure.MeteringClient for integration tests.
type stubMeteringClient struct {
	batchCalls int
	err        error
}

func (s *stubMeteringClient) BatchUsageEvent(_ context.Context, events []azurelicense.UsageEvent) (*azurelicense.BatchUsageResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.batchCalls++

	results := make([]azurelicense.UsageEventResult, 0, len(events))
	for range events {
		results = append(results, azurelicense.UsageEventResult{Status: "Accepted"})
	}

	return &azurelicense.BatchUsageResponse{
		Result: results,
		Count:  len(events),
	}, nil
}

func TestAzureIntegration_FullLifecycle(t *testing.T) {
	// Phase 1: Create provider with a known subscription.
	ffClient := &stubFulfillmentClient{
		subscription: &azurelicense.Subscription{
			SubscriptionID: "sub-001",
			PlanID:         azurelicense.PlanPro,
			Status:         azurelicense.StatusSubscribed,
			Purchaser: struct {
				EmailID  string `json:"emailId"`
				ObjectID string `json:"objectId"`
				TenantID string `json:"tenantId"`
			}{EmailID: "user@example.com"},
		},
	}

	provider, err := azurelicense.NewProvider(ffClient, "sub-001", azurelicense.PlanPro)
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
	metClient := &stubMeteringClient{}
	reporter := azurelicense.NewReporter(metClient, "sub-001", azurelicense.PlanPro)

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

	if metClient.batchCalls != 1 {
		t.Errorf("BatchUsageEvent calls = %d, want 1", metClient.batchCalls)
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

func TestAzureIntegration_LandingPageFlow(t *testing.T) {
	// Start without a subscription (pre-landing-page state).
	ffClient := &stubFulfillmentClient{
		resolved: &azurelicense.ResolvedSubscription{
			SubscriptionID: "sub-002",
			PlanID:         azurelicense.PlanEnterprise,
		},
		subscription: &azurelicense.Subscription{
			SubscriptionID: "sub-002",
			PlanID:         azurelicense.PlanEnterprise,
			Status:         azurelicense.StatusSubscribed,
			Purchaser: struct {
				EmailID  string `json:"emailId"`
				ObjectID string `json:"objectId"`
				TenantID string `json:"tenantId"`
			}{EmailID: "admin@corp.com"},
		},
	}

	provider, err := azurelicense.NewProvider(ffClient, "", "")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	// Before landing page: no subscription, uses paid defaults.
	if provider.SubscriptionID() != "" {
		t.Errorf("expected empty SubscriptionID before landing page, got %q", provider.SubscriptionID())
	}

	// Simulate landing page: resolve token and set subscription.
	resolved, err := ffClient.ResolveToken(context.Background(), "marketplace-token-xyz")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}

	if err := provider.SetSubscription(context.Background(), resolved.SubscriptionID, resolved.PlanID); err != nil {
		t.Fatalf("SetSubscription: %v", err)
	}

	// After landing page: subscription is set, entitlements updated.
	if provider.SubscriptionID() != "sub-002" {
		t.Errorf("SubscriptionID = %q, want %q", provider.SubscriptionID(), "sub-002")
	}

	info := provider.Info()
	if !info.Valid {
		t.Error("expected Valid = true after landing page")
	}
}

func TestAzureIntegration_SuspendedSubscription(t *testing.T) {
	ffClient := &stubFulfillmentClient{
		subscription: &azurelicense.Subscription{
			SubscriptionID: "sub-003",
			PlanID:         azurelicense.PlanPro,
			Status:         azurelicense.StatusSubscribed,
			Purchaser: struct {
				EmailID  string `json:"emailId"`
				ObjectID string `json:"objectId"`
				TenantID string `json:"tenantId"`
			}{EmailID: "user@example.com"},
		},
	}

	provider, err := azurelicense.NewProvider(ffClient, "sub-003", azurelicense.PlanPro)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if !provider.Info().Valid {
		t.Fatal("expected Valid = true initially")
	}

	// Simulate suspension via webhook.
	ffClient.subscription.Status = azurelicense.StatusSuspended
	provider.UpdateStatus(context.Background(), azurelicense.StatusSuspended, "")

	if provider.Info().Valid {
		t.Error("expected Valid = false after suspension")
	}
}

func TestAzureIntegration_RefreshFailure_KeepsLastGood(t *testing.T) {
	ffClient := &stubFulfillmentClient{
		subscription: &azurelicense.Subscription{
			SubscriptionID: "sub-004",
			PlanID:         azurelicense.PlanStarter,
			Status:         azurelicense.StatusSubscribed,
		},
	}

	provider, err := azurelicense.NewProvider(ffClient, "sub-004", azurelicense.PlanStarter)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if provider.Entitlements().MaxTenants != 5 {
		t.Fatalf("initial MaxTenants = %d, want 5", provider.Entitlements().MaxTenants)
	}

	// Simulate API failure.
	ffClient.getSubErr = errors.New("service unavailable")

	// Webhook-triggered refresh should fail but preserve last-known-good.
	provider.UpdateStatus(context.Background(), azurelicense.StatusSubscribed, "")

	if provider.Entitlements().MaxTenants != 5 {
		t.Errorf("MaxTenants = %d after error, want 5 (last-known-good)", provider.Entitlements().MaxTenants)
	}
}
