package gcp

import (
	"context"
	"errors"
	"testing"

	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
)

func TestNewProvider_ValidatesInputs(t *testing.T) {
	mock := &mockProcurementClient{}

	tests := []struct {
		name       string
		providerID string
		product    string
		wantErr    string
	}{
		{"empty provider ID", "", "my-product", "provider ID is required"},
		{"empty product", "my-provider", "", "product external name is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProvider(mock, tt.providerID, tt.product)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewProvider_InitialFetchSuccess(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{
				Entitlements: []*procurement.Entitlement{
					{
						ProductExternalName: "rbitr",
						State:               "ENTITLEMENT_ACTIVE",
						Plan:                PlanPro,
						Account:             "accounts/test-account",
					},
				},
			}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (pro plan)", p.Entitlements().MaxTenants)
	}

	info := p.Info()
	if !info.Valid {
		t.Error("expected Valid = true")
	}
	if info.Licensee != "accounts/test-account" {
		t.Errorf("Licensee = %q, want %q", info.Licensee, "accounts/test-account")
	}
}

func TestNewProvider_InitialFetchFailure_FallsBack(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return nil, errors.New("network error")
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still be valid with paid defaults.
	info := p.Info()
	if !info.Valid {
		t.Error("expected Valid = true even after fetch failure")
	}
}

func TestProvider_RefreshFindsActiveEntitlement(t *testing.T) {
	callCount := 0
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			callCount++
			if callCount == 1 {
				return &procurement.ListEntitlementsResponse{
					Entitlements: []*procurement.Entitlement{
						{
							ProductExternalName: "rbitr",
							State:               "ENTITLEMENT_ACTIVE",
							Plan:                PlanStarter,
							Account:             "accounts/starter-acct",
						},
					},
				}, nil
			}
			// Second call returns pro plan.
			return &procurement.ListEntitlementsResponse{
				Entitlements: []*procurement.Entitlement{
					{
						ProductExternalName: "rbitr",
						State:               "ENTITLEMENT_ACTIVE",
						Plan:                PlanPro,
						Account:             "accounts/starter-acct",
					},
				},
			}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Entitlements().MaxTenants != 5 {
		t.Errorf("MaxTenants = %d, want 5 (starter plan)", p.Entitlements().MaxTenants)
	}

	// Trigger refresh.
	if err := p.refresh(context.Background()); err != nil {
		t.Fatalf("refresh error: %v", err)
	}

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (pro plan after refresh)", p.Entitlements().MaxTenants)
	}
}

func TestProvider_RefreshKeepsLastKnownGood(t *testing.T) {
	callCount := 0
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			callCount++
			if callCount == 1 {
				return &procurement.ListEntitlementsResponse{
					Entitlements: []*procurement.Entitlement{
						{
							ProductExternalName: "rbitr",
							State:               "ENTITLEMENT_ACTIVE",
							Plan:                PlanPro,
							Account:             "accounts/test",
						},
					},
				}, nil
			}
			return nil, errors.New("API error")
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Refresh fails — should keep pro plan.
	_ = p.refresh(context.Background())

	if p.Entitlements().MaxTenants != 25 {
		t.Errorf("MaxTenants = %d, want 25 (should keep last-known-good)", p.Entitlements().MaxTenants)
	}
}

func TestProvider_SkipsWrongProduct(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{
				Entitlements: []*procurement.Entitlement{
					{
						ProductExternalName: "other-product",
						State:               "ENTITLEMENT_ACTIVE",
						Plan:                PlanEnterprise,
					},
				},
			}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have fallen back to defaults since no matching product.
	info := p.Info()
	if !info.Valid {
		t.Error("expected Valid = true")
	}
}

func TestProvider_SkipsCancelledEntitlement(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{
				Entitlements: []*procurement.Entitlement{
					{
						ProductExternalName: "rbitr",
						State:               "ENTITLEMENT_CANCELLED",
						Plan:                PlanEnterprise,
					},
				},
			}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to defaults since entitlement is cancelled.
	if p.AccountID() != "" {
		t.Errorf("AccountID = %q, want empty (cancelled entitlement)", p.AccountID())
	}
}

func TestProvider_PendingCancellationIsActive(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{
				Entitlements: []*procurement.Entitlement{
					{
						ProductExternalName: "rbitr",
						State:               "ENTITLEMENT_PENDING_CANCELLATION",
						Plan:                PlanPro,
						Account:             "accounts/pending",
					},
				},
			}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.AccountID() != "accounts/pending" {
		t.Errorf("AccountID = %q, want %q", p.AccountID(), "accounts/pending")
	}
}

func TestProvider_StartAndStop(t *testing.T) {
	mock := &mockProcurementClient{
		listFunc: func(_ string) (*procurement.ListEntitlementsResponse, error) {
			return &procurement.ListEntitlementsResponse{}, nil
		},
	}

	p, err := NewProvider(mock, "my-provider", "rbitr")
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

func TestIsActiveState(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"ENTITLEMENT_ACTIVE", true},
		{"ENTITLEMENT_PENDING_CANCELLATION", true},
		{"ENTITLEMENT_CANCELLED", false},
		{"ENTITLEMENT_ACTIVATION_REQUESTED", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := isActiveState(tt.state); got != tt.want {
				t.Errorf("isActiveState(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
