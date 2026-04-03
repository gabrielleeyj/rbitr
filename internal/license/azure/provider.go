package azure

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	// entitlementCacheTTL is how long entitlements are cached before re-fetching.
	entitlementCacheTTL = 5 * time.Minute

	// startupTimeout is how long the provider waits for the initial subscription
	// fetch before falling back to defaults.
	startupTimeout = 30 * time.Second
)

// Provider implements license.LicenseProvider using the Azure SaaS
// Fulfillment API v2. Subscriptions are polled periodically and cached.
type Provider struct {
	fulfillmentClient FulfillmentClient
	subscriptionID    string
	planID            string

	mu           sync.RWMutex
	entitlements license.Entitlements
	info         license.LicenseInfo
	status       string
	purchaser    string
}

var _ license.LicenseProvider = (*Provider)(nil)

// NewProvider creates a new Azure Marketplace provider. If subscriptionID is
// provided, it performs an initial subscription fetch. Otherwise it uses
// defaults until a landing page token resolves the subscription.
func NewProvider(fulfillClient FulfillmentClient, subscriptionID, planID string) (*Provider, error) {
	p := &Provider{
		fulfillmentClient: fulfillClient,
		subscriptionID:    subscriptionID,
		planID:            planID,
		entitlements:      license.PaidTierDefaults(),
	}

	if planID != "" {
		p.entitlements = mapPlanToEntitlements(planID)
	}
	p.info = p.buildInfo()

	if subscriptionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		if err := p.refresh(ctx); err != nil {
			slog.Warn("azure marketplace: initial subscription fetch failed, using plan defaults", "error", err)
		}
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

// Start begins the background subscription polling loop. Blocks until ctx is cancelled.
func (p *Provider) Start(ctx context.Context) {
	ticker := time.NewTicker(entitlementCacheTTL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.RLock()
			subID := p.subscriptionID
			p.mu.RUnlock()

			if subID == "" {
				continue
			}

			if err := p.refresh(ctx); err != nil {
				slog.Error("azure marketplace: subscription refresh failed, keeping last-known-good", "error", err)
			}
		}
	}
}

// SetSubscription updates the subscription ID and plan at runtime
// (e.g. after landing page token resolution). Triggers an immediate refresh.
func (p *Provider) SetSubscription(ctx context.Context, subscriptionID, planID string) error {
	p.mu.Lock()
	p.subscriptionID = subscriptionID
	if planID != "" {
		p.planID = planID
		p.entitlements = mapPlanToEntitlements(planID)
	}
	p.info = p.buildInfo()
	p.mu.Unlock()

	return p.refresh(ctx)
}

// UpdateStatus updates the subscription status from a webhook event.
func (p *Provider) UpdateStatus(ctx context.Context, newStatus, newPlanID string) {
	slog.Info("azure marketplace: subscription status change via webhook",
		"new_status", newStatus,
		"new_plan", newPlanID,
	)

	p.mu.Lock()
	p.status = newStatus
	if newPlanID != "" && newPlanID != p.planID {
		p.planID = newPlanID
		p.entitlements = mapPlanToEntitlements(newPlanID)
	}
	p.info = p.buildInfo()
	p.mu.Unlock()

	// Refresh from API to get authoritative state.
	if err := p.refresh(ctx); err != nil {
		slog.Error("azure marketplace: refresh after webhook failed", "error", err)
	}
}

// SubscriptionID returns the current subscription identifier.
func (p *Provider) SubscriptionID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.subscriptionID
}

// refresh fetches the subscription from the Azure SaaS API and updates the cache.
func (p *Provider) refresh(ctx context.Context) error {
	p.mu.RLock()
	subID := p.subscriptionID
	p.mu.RUnlock()

	if subID == "" {
		return errors.New("subscription ID not set")
	}

	sub, err := p.fulfillmentClient.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}

	ent := mapPlanToEntitlements(sub.PlanID)

	p.mu.Lock()
	p.entitlements = ent
	p.planID = sub.PlanID
	p.status = sub.Status
	p.purchaser = sub.Purchaser.EmailID
	p.info = p.buildInfo()
	p.mu.Unlock()

	slog.Info("azure marketplace: subscription refreshed",
		"subscription_id", subID,
		"plan", sub.PlanID,
		"status", sub.Status,
	)

	return nil
}

// buildInfo constructs a LicenseInfo from the current state.
func (p *Provider) buildInfo() license.LicenseInfo {
	tier := "paid"
	if p.planID == PlanStarter {
		tier = "starter"
	}

	valid := isActiveStatus(p.status) || p.status == ""
	if p.status == StatusSuspended {
		valid = false
	}

	return license.LicenseInfo{
		Valid:        valid,
		Tier:         tier,
		Licensee:     p.purchaser,
		Entitlements: p.entitlements,
	}
}
