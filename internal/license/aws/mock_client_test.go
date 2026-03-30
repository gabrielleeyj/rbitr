package aws_test

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	mptypes "github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"
)

// mockEntitlementClient is a mock of the EntitlementClient interface.
type mockEntitlementClient struct {
	getEntitlementsFunc func(ctx context.Context, params *marketplaceentitlementservice.GetEntitlementsInput, optFns ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error)
}

func (m *mockEntitlementClient) GetEntitlements(ctx context.Context, params *marketplaceentitlementservice.GetEntitlementsInput, optFns ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error) {
	if m.getEntitlementsFunc != nil {
		return m.getEntitlementsFunc(ctx, params, optFns...)
	}
	return &marketplaceentitlementservice.GetEntitlementsOutput{}, nil
}

// mockMeteringClient is a mock of the MeteringClient interface.
type mockMeteringClient struct {
	resolveCustomerFunc  func(ctx context.Context, params *marketplacemetering.ResolveCustomerInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.ResolveCustomerOutput, error)
	batchMeterUsageFunc  func(ctx context.Context, params *marketplacemetering.BatchMeterUsageInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error)
	batchMeterUsageCalls int
	lastBatchInput       *marketplacemetering.BatchMeterUsageInput
}

func (m *mockMeteringClient) ResolveCustomer(ctx context.Context, params *marketplacemetering.ResolveCustomerInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.ResolveCustomerOutput, error) {
	if m.resolveCustomerFunc != nil {
		return m.resolveCustomerFunc(ctx, params, optFns...)
	}
	return nil, errors.New("ResolveCustomer not configured")
}

func (m *mockMeteringClient) BatchMeterUsage(ctx context.Context, params *marketplacemetering.BatchMeterUsageInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error) {
	m.batchMeterUsageCalls++
	m.lastBatchInput = params
	if m.batchMeterUsageFunc != nil {
		return m.batchMeterUsageFunc(ctx, params, optFns...)
	}
	return &marketplacemetering.BatchMeterUsageOutput{
		Results:            make([]mptypes.UsageRecordResult, len(params.UsageRecords)),
		UnprocessedRecords: nil,
	}, nil
}
