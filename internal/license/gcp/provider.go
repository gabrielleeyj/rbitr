package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// entitlementCacheTTL is how long entitlements are cached before re-fetching.
	entitlementCacheTTL = 5 * time.Minute

	// startupTimeout is how long the provider waits for the initial entitlement
	// fetch before falling back to defaults.
	startupTimeout = 30 * time.Second

	// activeState is the GCP entitlement state indicating an active subscription.
	activeState = "ENTITLEMENT_ACTIVE"

	// pendingCancellationState still allows usage until end of billing cycle.
	pendingCancellationState = "ENTITLEMENT_PENDING_CANCELLATION"
)

// Provider implements license.LicenseProvider using the GCP Cloud Commerce
// Partner Procurement API. Entitlements are polled periodically and cached.
type Provider struct {
	procurementClient ProcurementClient
	providerID        string
	productExtName    string

	mu           sync.RWMutex
	entitlements license.Entitlements
	info         license.LicenseInfo
	accountID    string
	plan         string
}

var _ license.LicenseProvider = (*Provider)(nil)

// NewProvider creates a new GCP Marketplace provider. It performs an initial
// entitlement fetch; if the fetch fails, it falls back to paid defaults.
func NewProvider(procClient ProcurementClient, providerID, productExtName string) (*Provider, error) {
	if providerID == "" {
		return nil, errors.New("gcp marketplace: provider ID is required (derived from RBTR_GCP_PROJECT_ID)")
	}
	if productExtName == "" {
		return nil, errors.New("gcp marketplace: product external name is required (set RBTR_GCP_SERVICE_NAME)")
	}

	p := &Provider{
		procurementClient: procClient,
		providerID:        providerID,
		productExtName:    productExtName,
		entitlements:      license.PaidTierDefaults(),
	}
	p.info = p.buildInfo()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	if err := p.refresh(ctx); err != nil {
		slog.Warn("gcp marketplace: initial entitlement fetch failed, using paid defaults", "error", err)
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
				slog.Error("gcp marketplace: entitlement refresh failed, keeping last-known-good", "error", err)
			}
		}
	}
}

// UpdateEntitlement updates entitlements based on a webhook state change event.
// Called by the webhook handler when a Pub/Sub notification arrives.
func (p *Provider) UpdateEntitlement(ctx context.Context, entitlementID, newState string) {
	slog.Info("gcp marketplace: entitlement state change via webhook",
		"entitlement_id", entitlementID,
		"new_state", newState,
	)

	// Trigger an immediate refresh to get the latest state from the API.
	if err := p.refresh(ctx); err != nil {
		slog.Error("gcp marketplace: refresh after webhook failed", "error", err)
	}
}

// AccountID returns the current GCP account identifier.
func (p *Provider) AccountID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.accountID
}

// refresh fetches entitlements from the GCP Procurement API and updates the cache.
func (p *Provider) refresh(_ context.Context) error {
	parent := "providers/" + p.providerID

	resp, err := p.procurementClient.ListEntitlements(parent)
	if err != nil {
		return fmt.Errorf("ListEntitlements: %w", err)
	}

	// Find the active entitlement matching our product.
	for _, ent := range resp.Entitlements {
		if ent.ProductExternalName != p.productExtName {
			continue
		}

		if !isActiveState(ent.State) {
			continue
		}

		mapped := mapPlanToEntitlements(ent.Plan, json.RawMessage(ent.InputProperties))

		p.mu.Lock()
		p.entitlements = mapped
		p.plan = ent.Plan
		p.accountID = ent.Account
		p.info = p.buildInfo()
		p.mu.Unlock()

		slog.Info("gcp marketplace: entitlements refreshed",
			"account", ent.Account,
			"plan", ent.Plan,
			"state", ent.State,
		)

		return nil
	}

	return errors.New("no active entitlement found for product")
}

// buildInfo constructs a LicenseInfo from the current state.
func (p *Provider) buildInfo() license.LicenseInfo {
	tier := "paid"
	if p.plan == PlanStarter {
		tier = "starter"
	}
	return license.LicenseInfo{
		Valid:        true,
		Tier:         tier,
		Licensee:     p.accountID,
		Entitlements: p.entitlements,
	}
}

// isActiveState returns true if the entitlement state allows usage.
func isActiveState(state string) bool {
	return state == activeState || state == pendingCancellationState
}
