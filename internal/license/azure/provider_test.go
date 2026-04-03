package azure

import (
	"context"
	"errors"
	"testing"
)

func TestNewProvider_NoPlanDefaults(t *testing.T) {
	mock := &mockFulfillmentClient{}

	p, err := NewProvider(mock, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info := p.Info()
	if !info.Valid {
		t.Error("expected Valid = true")
	}
}

func TestNewProvider_WithPlanID(t *testing.T) {
	mock := &mockFulfillmentClient{}

	p, err := NewProvider(mock, "", PlanPro)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (pro plan)", p.Entitlements().MaxTenants)
	}
}

func TestNewProvider_WithSubscriptionID_FetchesSubscription(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{
				PlanID: PlanStarter,
				Status: StatusSubscribed,
				Purchaser: struct {
					EmailID  string `json:"emailId"`
					ObjectID string `json:"objectId"`
					TenantID string `json:"tenantId"`
				}{EmailID: "user@example.com"},
			}, nil
		},
	}

	p, err := NewProvider(mock, "sub-123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Entitlements().MaxTenants != 5 {
		t.Errorf("MaxTenants = %d, want 5 (starter plan from API)", p.Entitlements().MaxTenants)
	}

	info := p.Info()
	if !info.Valid {
		t.Error("expected Valid = true for Subscribed status")
	}
	if info.Licensee != "user@example.com" {
		t.Errorf("Licensee = %q, want %q", info.Licensee, "user@example.com")
	}
}

func TestNewProvider_FetchFailure_FallsBack(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return nil, errors.New("network error")
		},
	}

	p, err := NewProvider(mock, "sub-123", PlanPro)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still use the plan defaults.
	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (pro plan defaults)", p.Entitlements().MaxTenants)
	}
}

func TestProvider_RefreshUpdatesEntitlements(t *testing.T) {
	callCount := 0
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			callCount++
			if callCount == 1 {
				return &Subscription{PlanID: PlanStarter, Status: StatusSubscribed}, nil
			}
			return &Subscription{PlanID: PlanPro, Status: StatusSubscribed}, nil
		},
	}

	p, err := NewProvider(mock, "sub-123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Entitlements().MaxTenants != 5 {
		t.Errorf("MaxTenants = %d, want 5 (starter)", p.Entitlements().MaxTenants)
	}

	if err := p.refresh(context.Background()); err != nil {
		t.Fatalf("refresh error: %v", err)
	}

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (pro after refresh)", p.Entitlements().MaxTenants)
	}
}

func TestProvider_RefreshKeepsLastKnownGood(t *testing.T) {
	callCount := 0
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			callCount++
			if callCount == 1 {
				return &Subscription{PlanID: PlanPro, Status: StatusSubscribed}, nil
			}
			return nil, errors.New("API error")
		},
	}

	p, err := NewProvider(mock, "sub-123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = p.refresh(context.Background())

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (should keep last-known-good)", p.Entitlements().MaxTenants)
	}
}

func TestProvider_SetSubscription(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanEnterprise, Status: StatusSubscribed}, nil
		},
	}

	p, err := NewProvider(mock, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := p.SetSubscription(context.Background(), "sub-456", PlanEnterprise); err != nil {
		t.Fatalf("SetSubscription error: %v", err)
	}

	if p.SubscriptionID() != "sub-456" {
		t.Errorf("SubscriptionID = %q, want %q", p.SubscriptionID(), "sub-456")
	}
}

func TestProvider_UpdateStatus_Suspended(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusSuspended}, nil
		},
	}

	p, err := NewProvider(mock, "sub-123", PlanPro)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.UpdateStatus(context.Background(), StatusSuspended, "")

	info := p.Info()
	if info.Valid {
		t.Error("expected Valid = false for suspended subscription")
	}
}

func TestProvider_StartAndStop(t *testing.T) {
	mock := &mockFulfillmentClient{
		getSubFunc: func(_ context.Context, _ string) (*Subscription, error) {
			return &Subscription{PlanID: PlanPro, Status: StatusSubscribed}, nil
		},
	}

	p, err := NewProvider(mock, "sub-123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Start(ctx)
		close(done)
	}()

	cancel()
	<-done
}
