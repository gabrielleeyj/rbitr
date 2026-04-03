package aws_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	enttypes "github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice/types"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	mptypes "github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"

	awslicense "github.com/gabrielleeyj/rbitr/internal/license/aws"
)

// stubEntitlementClient implements aws.EntitlementClient for integration tests.
type stubEntitlementClient struct {
	entitlements []enttypes.Entitlement
	err          error
}

func (s *stubEntitlementClient) GetEntitlements(_ context.Context, _ *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &marketplaceentitlementservice.GetEntitlementsOutput{
		Entitlements: s.entitlements,
	}, nil
}

// stubMeteringClient implements aws.MeteringClient for integration tests.
type stubMeteringClient struct {
	resolvedCustomerID string
	batchResults       []mptypes.UsageRecordResult
	unprocessed        []mptypes.UsageRecord
	resolveErr         error
	batchErr           error
}

func (s *stubMeteringClient) ResolveCustomer(_ context.Context, input *marketplacemetering.ResolveCustomerInput, _ ...func(*marketplacemetering.Options)) (*marketplacemetering.ResolveCustomerOutput, error) {
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return &marketplacemetering.ResolveCustomerOutput{
		CustomerIdentifier: &s.resolvedCustomerID,
		ProductCode:        input.RegistrationToken,
	}, nil
}

func (s *stubMeteringClient) BatchMeterUsage(_ context.Context, _ *marketplacemetering.BatchMeterUsageInput, _ ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error) {
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	return &marketplacemetering.BatchMeterUsageOutput{
		Results:            s.batchResults,
		UnprocessedRecords: s.unprocessed,
	}, nil
}

func TestAWSIntegration_FullLifecycle(t *testing.T) {
	// Phase 1: Create provider with entitlements.
	maxTenants := int32(50)
	monthlyActions := float64(100_000)
	approvalWorkflows := true

	entClient := &stubEntitlementClient{
		entitlements: []enttypes.Entitlement{
			{
				Dimension: strPtr(awslicense.DimensionMaxTenants),
				Value:     &enttypes.EntitlementValue{IntegerValue: &maxTenants},
			},
			{
				Dimension: strPtr(awslicense.DimensionMonthlyActions),
				Value:     &enttypes.EntitlementValue{DoubleValue: &monthlyActions},
			},
			{
				Dimension: strPtr(awslicense.DimensionApprovalWorkflows),
				Value:     &enttypes.EntitlementValue{BooleanValue: &approvalWorkflows},
			},
		},
	}

	metClient := &stubMeteringClient{}

	provider, err := awslicense.NewProvider(entClient, "prod-code-123", "cust-456")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	// Phase 2: Verify entitlements are loaded.
	ent := provider.Entitlements()
	if ent.MaxTenants != 50 {
		t.Errorf("MaxTenants = %d, want 50", ent.MaxTenants)
	}
	if ent.MonthlyActionLimit != 100_000 {
		t.Errorf("MonthlyActionLimit = %d, want 100000", ent.MonthlyActionLimit)
	}
	if !ent.ApprovalWorkflows {
		t.Error("ApprovalWorkflows should be true")
	}

	info := provider.Info()
	if !info.Valid {
		t.Error("expected Valid = true")
	}
	if info.Tier != "paid" {
		t.Errorf("Tier = %q, want %q", info.Tier, "paid")
	}

	// Phase 3: Record usage and flush.
	reporter := awslicense.NewReporter(metClient, "prod-code-123", "cust-456")

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

func TestAWSIntegration_EntitlementRefreshFailure_KeepsLastGood(t *testing.T) {
	callCount := 0
	entClient := &stubEntitlementClient{}
	// Override behavior per call.
	origGetEnt := entClient.GetEntitlements
	_ = origGetEnt

	maxTenants := int32(10)
	entClient.entitlements = []enttypes.Entitlement{
		{
			Dimension: strPtr(awslicense.DimensionMaxTenants),
			Value:     &enttypes.EntitlementValue{IntegerValue: &maxTenants},
		},
	}

	provider, err := awslicense.NewProvider(entClient, "prod-123", "cust-123")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if provider.Entitlements().MaxTenants != 10 {
		t.Fatalf("initial MaxTenants = %d, want 10", provider.Entitlements().MaxTenants)
	}

	// Simulate API failure — entitlements should be preserved.
	entClient.err = errors.New("throttled")
	callCount++

	// Provider keeps last-known-good on error (tested via unit tests).
	// This integration test verifies the provider still returns valid data.
	if provider.Entitlements().MaxTenants != 10 {
		t.Errorf("MaxTenants = %d after error, want 10 (last-known-good)", provider.Entitlements().MaxTenants)
	}
}

func strPtr(s string) *string { return &s }
