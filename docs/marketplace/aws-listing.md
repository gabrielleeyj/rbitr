# AWS Marketplace Listing Guide

## Overview

rbitr integrates with AWS Marketplace via the **Entitlement Service** (for license
entitlements) and the **Metering Service** (for usage reporting). Customers subscribe
through the AWS Marketplace console and rbitr automatically provisions entitlements.

## Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌──────────────┐
│  AWS Marketplace │────▶│  Entitlement Service │────▶│   rbitr      │
│  (customer sub)  │     │  (GetEntitlements)   │     │   Provider   │
└─────────────────┘     └──────────────────────┘     └──────────────┘
                                                            │
                        ┌──────────────────────┐            │
                        │  Metering Service    │◀───────────┘
                        │  (BatchMeterUsage)   │     Reporter
                        └──────────────────────┘
```

## Entitlement Dimensions

| Dimension             | Type    | Description                          |
|-----------------------|---------|--------------------------------------|
| `max_tenants`         | Integer | Maximum tenant count                 |
| `monthly_actions`     | Double  | Monthly governed action quota         |
| `approval_workflows`  | Boolean | Approval workflows feature flag      |

## Prerequisites

1. **AWS Marketplace Seller Account** — Register at [AWS Marketplace Management Portal](https://aws.amazon.com/marketplace/management/).
2. **Product Listing** — Create a SaaS product listing with the entitlement dimensions above.
3. **IAM Role** — The EKS service account needs permissions for:
   - `aws-marketplace:GetEntitlements`
   - `aws-marketplace:BatchMeterUsage`
   - `aws-marketplace:ResolveCustomer`

## Deployment

### Helm Values

```yaml
license:
  provider: "aws-marketplace"

aws:
  productCode: "prod-abc123"
  region: "us-east-1"        # optional, defaults to SDK default
  customerID: ""              # optional, resolved via activation endpoint
```

Or use the overlay file:

```bash
helm install rbitr ./deploy/helm/rbitr \
  -f deploy/helm/rbitr/values-aws-marketplace.yaml \
  --set aws.productCode=prod-abc123
```

### IRSA (IAM Roles for Service Accounts)

```yaml
serviceAccount:
  create: true
  annotations:
    eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/rbitr-marketplace"
```

### Environment Variables

| Variable               | Required | Description                      |
|------------------------|----------|----------------------------------|
| `RBTR_LICENSE_PROVIDER` | Yes     | Set to `aws-marketplace`         |
| `RBTR_AWS_PRODUCT_CODE` | Yes     | AWS Marketplace product code     |
| `RBTR_AWS_REGION`       | No      | AWS region override              |
| `RBTR_AWS_CUSTOMER_ID`  | No      | Pre-resolved customer identifier |

## Customer Activation Flow

1. Customer subscribes via AWS Marketplace.
2. AWS redirects to rbitr's activation endpoint with a registration token.
3. rbitr calls `ResolveCustomer` to map the token to a customer ID.
4. Entitlements are fetched via `GetEntitlements` and cached.

### Activation Endpoint

```
POST /api/marketplace/aws/activate
Content-Type: application/json

{ "registration_token": "<token-from-aws>" }
```

## Usage Metering

rbitr reports usage to AWS Marketplace hourly via `BatchMeterUsage`. Usage records
are buffered in memory and flushed periodically. The dimension reported is
`governed_actions` with the quantity of actions performed per tenant per hour.

- **Flush interval**: 1 hour
- **Max batch size**: 25 records
- **Early flush threshold**: 20 records (triggers immediate flush)

## Monitoring

- Check gateway logs for `aws marketplace:` prefixed messages.
- Entitlement refresh failures are logged at ERROR level but do not cause downtime
  (last-known-good entitlements are preserved).
- Usage metering failures are re-buffered for retry on the next flush cycle.

## Troubleshooting

| Symptom                        | Cause                           | Fix                                    |
|--------------------------------|---------------------------------|----------------------------------------|
| Free tier despite subscription | Missing product code            | Set `RBTR_AWS_PRODUCT_CODE`            |
| Entitlement fetch timeout      | IRSA misconfigured              | Verify service account IAM role        |
| Usage not appearing            | Customer ID not resolved        | Trigger activation endpoint            |
| "throttled" errors in logs     | AWS API rate limiting           | Entitlements retry on next poll cycle   |
