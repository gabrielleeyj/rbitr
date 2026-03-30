package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
)

// EntitlementClient abstracts the AWS Marketplace Entitlement Service API.
// The real implementation is *marketplaceentitlementservice.Client.
type EntitlementClient interface {
	GetEntitlements(ctx context.Context, params *marketplaceentitlementservice.GetEntitlementsInput, optFns ...func(*marketplaceentitlementservice.Options)) (*marketplaceentitlementservice.GetEntitlementsOutput, error)
}

// MeteringClient abstracts the AWS Marketplace Metering Service API.
// The real implementation is *marketplacemetering.Client.
type MeteringClient interface {
	ResolveCustomer(ctx context.Context, params *marketplacemetering.ResolveCustomerInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.ResolveCustomerOutput, error)
	BatchMeterUsage(ctx context.Context, params *marketplacemetering.BatchMeterUsageInput, optFns ...func(*marketplacemetering.Options)) (*marketplacemetering.BatchMeterUsageOutput, error)
}
