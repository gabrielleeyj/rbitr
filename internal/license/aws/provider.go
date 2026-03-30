package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// entitlementCacheTTL is how long entitlements are cached before re-fetching.
	entitlementCacheTTL = 5 * time.Minute

	// startupTimeout is how long the provider waits for the initial entitlement
	// fetch before returning an error.
	startupTimeout = 30 * time.Second
)

// Provider implements license.LicenseProvider using AWS Marketplace
// Entitlement Service. Entitlements are polled periodically and cached.
type Provider struct {
	entitlementClient EntitlementClient
	productCode       string
	customerID        string

	mu           sync.RWMutex
	entitlements license.Entitlements
	info         license.LicenseInfo
}

var _ license.LicenseProvider = (*Provider)(nil)

// NewProvider creates a new AWS Marketplace provider. It performs an initial
// entitlement fetch and returns an error if the fetch fails.
func NewProvider(entClient EntitlementClient, productCode, customerID string) (*Provider, error) {
	if productCode == "" {
		return nil, errors.New("aws marketplace: product code is required (set RBTR_AWS_PRODUCT_CODE)")
	}
	if customerID == "" {
		return nil, errors.New("aws marketplace: customer ID is required (set RBTR_AWS_CUSTOMER_ID or resolve via activation endpoint)")
	}

	p := &Provider{
		entitlementClient: entClient,
		productCode:       productCode,
		customerID:        customerID,
		entitlements:      license.PaidTierDefaults(),
	}
	p.info = p.buildInfo()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	if err := p.refresh(ctx); err != nil {
		slog.Warn("aws marketplace: initial entitlement fetch failed, using paid defaults", "error", err)
	}

	return p, nil
}

// Entitlements returns the cached entitlements (thread-safe).
func (p *Provider) Entitlements() license.Entitlements {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.entitlements
}

// Info returns the current license metadata.
func (p *Provider) Info() license.LicenseInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.info
}

// Start begins the background entitlement polling loop. Blocks until ctx is cancelled.
func (p *Provider) Start(ctx context.Context) {
	ticker := time.NewTicker(entitlementCacheTTL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.refresh(ctx); err != nil {
				slog.Error("aws marketplace: entitlement refresh failed, keeping last-known-good", "error", err)
			}
		}
	}
}

// SetCustomerID updates the customer ID at runtime (e.g. after token resolution).
// Triggers an immediate entitlement refresh.
func (p *Provider) SetCustomerID(ctx context.Context, customerID string) error {
	p.mu.Lock()
	p.customerID = customerID
	p.mu.Unlock()

	return p.refresh(ctx)
}

// CustomerID returns the current customer identifier.
func (p *Provider) CustomerID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.customerID
}

// refresh fetches entitlements from AWS and updates the cache.
func (p *Provider) refresh(ctx context.Context) error {
	p.mu.RLock()
	custID := p.customerID
	p.mu.RUnlock()

	if custID == "" {
		return errors.New("customer ID not set")
	}

	input := &marketplaceentitlementservice.GetEntitlementsInput{
		ProductCode: &p.productCode,
		Filter: map[string][]string{
			"CUSTOMER_IDENTIFIER": {custID},
		},
	}

	output, err := p.entitlementClient.GetEntitlements(ctx, input)
	if err != nil {
		return fmt.Errorf("GetEntitlements: %w", err)
	}

	ent := mapDimensionsToEntitlements(output.Entitlements)

	p.mu.Lock()
	p.entitlements = ent
	p.info = p.buildInfo()
	p.mu.Unlock()

	slog.Info("aws marketplace: entitlements refreshed",
		"customer_id", custID,
		"tier", ent.Tier,
		"entitlement_count", len(output.Entitlements),
	)

	return nil
}

// buildInfo constructs a LicenseInfo from the current state. Must be called
// with at least an RLock held (or during construction).
func (p *Provider) buildInfo() license.LicenseInfo {
	return license.LicenseInfo{
		Valid:        true,
		Tier:         "paid",
		Licensee:     p.customerID,
		Entitlements: p.entitlements,
	}
}
