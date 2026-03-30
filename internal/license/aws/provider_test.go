package aws_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awslicense "github.com/gabrielleeyj/rbitr/internal/license/aws"
)

func TestNewProvider_Validation(t *testing.T) {
	t.Parallel()

	t.Run("empty product code returns error", func(t *testing.T) {
		t.Parallel()
		_, err := awslicense.NewProvider(&mockEntitlementClient{}, "", "customer-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "product code is required")
	})

	t.Run("empty customer ID returns error", func(t *testing.T) {
		t.Parallel()
		_, err := awslicense.NewProvider(&mockEntitlementClient{}, "prod-1", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "customer ID is required")
	})
}

func TestNewProvider_InitialFetch(t *testing.T) {
	t.Parallel()

	t.Run("successful initial fetch maps entitlements", func(t *testing.T) {
		t.Parallel()

		entClient := &mockEntitlementClient{
			getEntitlementsFunc: func(_ context.Context, input *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
				assert.Equal(t, "prod-1", *input.ProductCode)
				return &marketplaceentitlementservice.GetEntitlementsOutput{
					Entitlements: []types.Entitlement{
						{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{IntegerValue: ptr(int32(20))}},
					},
				}, nil
			},
		}

		provider, err := awslicense.NewProvider(entClient, "prod-1", "cust-1")
		require.NoError(t, err)

		assert.Equal(t, 20, provider.Entitlements().MaxTenants)
	})

	t.Run("failed initial fetch uses paid defaults", func(t *testing.T) {
		t.Parallel()

		entClient := &mockEntitlementClient{
			getEntitlementsFunc: func(_ context.Context, _ *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
				return nil, errors.New("network error")
			},
		}

		provider, err := awslicense.NewProvider(entClient, "prod-1", "cust-1")
		require.NoError(t, err) // Should not error — falls back to defaults

		ent := provider.Entitlements()
		assert.Equal(t, "paid", ent.Tier)
	})
}

func TestProvider_Info(t *testing.T) {
	t.Parallel()

	entClient := &mockEntitlementClient{}
	provider, err := awslicense.NewProvider(entClient, "prod-1", "cust-1")
	require.NoError(t, err)

	info := provider.Info()
	assert.True(t, info.Valid)
	assert.Equal(t, "paid", info.Tier)
	assert.Equal(t, "cust-1", info.Licensee)
}

func TestProvider_StartAndStop(t *testing.T) {
	t.Parallel()

	callCount := 0
	entClient := &mockEntitlementClient{
		getEntitlementsFunc: func(_ context.Context, _ *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
			callCount++
			return &marketplaceentitlementservice.GetEntitlementsOutput{}, nil
		},
	}

	provider, err := awslicense.NewProvider(entClient, "prod-1", "cust-1")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		provider.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Start returned after context cancellation — success
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestProvider_SetCustomerID(t *testing.T) {
	t.Parallel()

	entClient := &mockEntitlementClient{
		getEntitlementsFunc: func(_ context.Context, input *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
			return &marketplaceentitlementservice.GetEntitlementsOutput{
				Entitlements: []types.Entitlement{
					{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{IntegerValue: ptr(int32(30))}},
				},
			}, nil
		},
	}

	provider, err := awslicense.NewProvider(entClient, "prod-1", "initial-cust")
	require.NoError(t, err)

	err = provider.SetCustomerID(context.Background(), "new-cust")
	require.NoError(t, err)

	assert.Equal(t, "new-cust", provider.CustomerID())
	assert.Equal(t, "new-cust", provider.Info().Licensee)
}

func TestProvider_ErrorFallback(t *testing.T) {
	t.Parallel()

	callCount := 0
	entClient := &mockEntitlementClient{
		getEntitlementsFunc: func(_ context.Context, _ *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
			callCount++
			if callCount == 1 {
				return &marketplaceentitlementservice.GetEntitlementsOutput{
					Entitlements: []types.Entitlement{
						{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{IntegerValue: ptr(int32(42))}},
					},
				}, nil
			}
			return nil, errors.New("API unavailable")
		},
	}

	provider, err := awslicense.NewProvider(entClient, "prod-1", "cust-1")
	require.NoError(t, err)
	assert.Equal(t, 42, provider.Entitlements().MaxTenants)

	// Second call (via SetCustomerID) fails — should keep last-known-good
	_ = provider.SetCustomerID(context.Background(), "cust-1")
	assert.Equal(t, 42, provider.Entitlements().MaxTenants)
}
