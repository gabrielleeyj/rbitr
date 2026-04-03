package azure

import (
	"context"
	"sync"
)

// mockFulfillmentClient is a test double for FulfillmentClient.
type mockFulfillmentClient struct {
	mu                  sync.Mutex
	resolveFunc         func(ctx context.Context, token string) (*ResolvedSubscription, error)
	getSubFunc          func(ctx context.Context, subID string) (*Subscription, error)
	activateFunc        func(ctx context.Context, subID, planID string) error
	listFunc            func(ctx context.Context) ([]Subscription, error)
	resolveCallCount    int
	getSubCallCount     int
	activateCallCount   int
	lastActivatedSubID  string
	lastActivatedPlanID string
}

func (m *mockFulfillmentClient) ResolveToken(ctx context.Context, token string) (*ResolvedSubscription, error) {
	m.mu.Lock()
	m.resolveCallCount++
	m.mu.Unlock()

	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, token)
	}
	return &ResolvedSubscription{}, nil
}

func (m *mockFulfillmentClient) GetSubscription(ctx context.Context, subID string) (*Subscription, error) {
	m.mu.Lock()
	m.getSubCallCount++
	m.mu.Unlock()

	if m.getSubFunc != nil {
		return m.getSubFunc(ctx, subID)
	}
	return &Subscription{}, nil
}

func (m *mockFulfillmentClient) ActivateSubscription(ctx context.Context, subID, planID string) error {
	m.mu.Lock()
	m.activateCallCount++
	m.lastActivatedSubID = subID
	m.lastActivatedPlanID = planID
	m.mu.Unlock()

	if m.activateFunc != nil {
		return m.activateFunc(ctx, subID, planID)
	}
	return nil
}

func (m *mockFulfillmentClient) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

// mockMeteringClient is a test double for MeteringClient.
type mockMeteringClient struct {
	mu           sync.Mutex
	batchFunc    func(ctx context.Context, events []UsageEvent) (*BatchUsageResponse, error)
	batchCalls   int
	lastBatchReq []UsageEvent
}

func (m *mockMeteringClient) BatchUsageEvent(ctx context.Context, events []UsageEvent) (*BatchUsageResponse, error) {
	m.mu.Lock()
	m.batchCalls++
	m.lastBatchReq = events
	m.mu.Unlock()

	if m.batchFunc != nil {
		return m.batchFunc(ctx, events)
	}
	return &BatchUsageResponse{
		Result: make([]UsageEventResult, len(events)),
		Count:  len(events),
	}, nil
}
