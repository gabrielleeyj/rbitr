package azure

import (
	"context"
	"time"
)

// Subscription represents an Azure Marketplace SaaS subscription.
type Subscription struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscriptionId"`
	OfferID        string `json:"offerId"`
	PlanID         string `json:"planId"`
	Name           string `json:"name"`
	Status         string `json:"saasSubscriptionStatus"`
	Purchaser      struct {
		EmailID  string `json:"emailId"`
		ObjectID string `json:"objectId"`
		TenantID string `json:"tenantId"`
	} `json:"purchaser"`
	Beneficiary struct {
		EmailID  string `json:"emailId"`
		ObjectID string `json:"objectId"`
		TenantID string `json:"tenantId"`
	} `json:"beneficiary"`
	Term struct {
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	} `json:"term"`
}

// ResolvedSubscription is returned by the SaaS Fulfillment resolve endpoint.
type ResolvedSubscription struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscriptionName"`
	OfferID        string `json:"offerId"`
	PlanID         string `json:"planId"`
	Quantity       int    `json:"quantity"`
}

// UsageEvent represents a single usage report for the Marketplace Metering API.
type UsageEvent struct {
	ResourceID    string    `json:"resourceId"`
	Quantity      float64   `json:"quantity"`
	Dimension     string    `json:"dimension"`
	EffectiveTime time.Time `json:"effectiveStartTime"`
	PlanID        string    `json:"planId"`
}

// UsageEventResult is the response for a single usage event.
type UsageEventResult struct {
	Status        string  `json:"status"`
	MessageTime   string  `json:"messageTime"`
	ResourceID    string  `json:"resourceId"`
	Quantity      float64 `json:"quantity"`
	Dimension     string  `json:"dimension"`
	EffectiveTime string  `json:"effectiveStartTime"`
	PlanID        string  `json:"planId"`
}

// BatchUsageResponse is the response from the batch metering endpoint.
type BatchUsageResponse struct {
	Result []UsageEventResult `json:"result"`
	Count  int                `json:"count"`
}

// FulfillmentClient abstracts the Azure SaaS Fulfillment API v2.
type FulfillmentClient interface {
	// ResolveToken resolves a marketplace purchase token to a subscription.
	ResolveToken(ctx context.Context, token string) (*ResolvedSubscription, error)

	// GetSubscription retrieves the current state of a subscription.
	GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error)

	// ActivateSubscription activates a pending subscription.
	ActivateSubscription(ctx context.Context, subscriptionID, planID string) error

	// ListSubscriptions lists all SaaS subscriptions.
	ListSubscriptions(ctx context.Context) ([]Subscription, error)
}

// MeteringClient abstracts the Azure Marketplace Metering API.
type MeteringClient interface {
	// BatchUsageEvent reports a batch of usage events.
	BatchUsageEvent(ctx context.Context, events []UsageEvent) (*BatchUsageResponse, error)
}
