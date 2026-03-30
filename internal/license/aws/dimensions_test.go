package aws_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice/types"
	"github.com/stretchr/testify/assert"

	awslicense "github.com/gabrielleeyj/rbitr/internal/license/aws"
)

func ptr[T any](v T) *T { return &v }

func TestMapDimensionsToEntitlements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dimensions []types.Entitlement
		check      func(t *testing.T, provider *awslicense.Provider)
	}{
		{
			name:       "empty dimensions returns paid defaults",
			dimensions: nil,
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.Equal(t, "paid", ent.Tier)
			},
		},
		{
			name: "all integer dimensions mapped",
			dimensions: []types.Entitlement{
				{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{IntegerValue: ptr(int32(50))}},
				{Dimension: ptr(awslicense.DimensionMaxAgentsPerTenant), Value: &types.EntitlementValue{IntegerValue: ptr(int32(10))}},
				{Dimension: ptr(awslicense.DimensionMaxActiveKeys), Value: &types.EntitlementValue{IntegerValue: ptr(int32(100))}},
				{Dimension: ptr(awslicense.DimensionMonthlyActions), Value: &types.EntitlementValue{IntegerValue: ptr(int32(500000))}},
				{Dimension: ptr(awslicense.DimensionAuditRetentionDays), Value: &types.EntitlementValue{IntegerValue: ptr(int32(365))}},
			},
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.Equal(t, 50, ent.MaxTenants)
				assert.Equal(t, 10, ent.MaxAgentsPerTenant)
				assert.Equal(t, 100, ent.MaxActiveKeys)
				assert.Equal(t, int64(500000), ent.MonthlyActionLimit)
				assert.Equal(t, 365, ent.AuditRetentionDays)
			},
		},
		{
			name: "boolean dimensions mapped",
			dimensions: []types.Entitlement{
				{Dimension: ptr(awslicense.DimensionApprovalWorkflows), Value: &types.EntitlementValue{BooleanValue: ptr(true)}},
				{Dimension: ptr(awslicense.DimensionEvidenceExport), Value: &types.EntitlementValue{BooleanValue: ptr(false)}},
				{Dimension: ptr(awslicense.DimensionIntegrations), Value: &types.EntitlementValue{BooleanValue: ptr(true)}},
				{Dimension: ptr(awslicense.DimensionCustomPolicies), Value: &types.EntitlementValue{BooleanValue: ptr(true)}},
			},
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.True(t, ent.ApprovalWorkflows)
				assert.False(t, ent.EvidenceExport)
				assert.True(t, ent.Integrations)
				assert.True(t, ent.CustomPolicies)
			},
		},
		{
			name: "nil dimension and value are skipped",
			dimensions: []types.Entitlement{
				{Dimension: nil, Value: &types.EntitlementValue{IntegerValue: ptr(int32(5))}},
				{Dimension: ptr(awslicense.DimensionMaxTenants), Value: nil},
			},
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.Equal(t, "paid", ent.Tier)
			},
		},
		{
			name: "double value falls back correctly",
			dimensions: []types.Entitlement{
				{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{DoubleValue: ptr(float64(25))}},
			},
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.Equal(t, 25, ent.MaxTenants)
			},
		},
		{
			name: "unknown dimensions are ignored",
			dimensions: []types.Entitlement{
				{Dimension: ptr("unknown_dimension"), Value: &types.EntitlementValue{IntegerValue: ptr(int32(99))}},
				{Dimension: ptr(awslicense.DimensionMaxTenants), Value: &types.EntitlementValue{IntegerValue: ptr(int32(42))}},
			},
			check: func(t *testing.T, p *awslicense.Provider) {
				ent := p.Entitlements()
				assert.Equal(t, 42, ent.MaxTenants)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entClient := &mockEntitlementClient{
				getEntitlementsFunc: func(_ context.Context, _ *marketplaceentitlementservice.GetEntitlementsInput, _ ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
					return &marketplaceentitlementservice.GetEntitlementsOutput{
						Entitlements: tt.dimensions,
					}, nil
				},
			}

			provider, err := awslicense.NewProvider(entClient, "test-product", "test-customer")
			assert.NoError(t, err)

			tt.check(t, provider)
		})
	}
}
