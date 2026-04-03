# Azure Marketplace Listing Guide

## Overview

rbitr integrates with Azure Marketplace via the **SaaS Fulfillment API v2** (for
subscription management) and the **Marketplace Metering API** (for usage reporting).
Customers land on a registration page after purchasing, where the subscription is
resolved and activated.

## Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌──────────────┐
│  Azure Market.   │────▶│  Landing Page        │────▶│   rbitr      │
│  (customer buy)  │     │  GET /api/mp/az/land │     │   Provider   │
└─────────────────┘     └──────────────────────┘     └──────────────┘
        │                                                   │
        │ Webhook          ┌──────────────────────┐         │
        └─────────────────▶│  Lifecycle Webhook   │         │
                           │  POST /api/mp/az/wh  │         │
                           └──────────────────────┘         │
                           ┌──────────────────────┐         │
                           │  Metering API        │◀────────┘
                           │  (BatchUsageEvent)   │  Reporter
                           └──────────────────────┘
```

## Plan Tiers

| Plan         | MaxTenants | Agents/Tenant | MonthlyActions | Approval | Evidence |
|--------------|------------|---------------|----------------|----------|----------|
| `starter`    | 5          | 10            | 10,000         | No       | No       |
| `pro`        | 25         | 50            | 100,000        | Yes      | Yes      |
| `enterprise` | Unlimited  | Unlimited     | Unlimited      | Yes      | Yes      |

## Prerequisites

1. **Partner Center Account** — Register at [Microsoft Partner Center](https://partner.microsoft.com/).
2. **SaaS Offer** — Create a SaaS offer with the plan tiers above.
3. **Azure AD App Registration** — Required for API authentication:
   - Application (client) ID
   - Client secret
   - Tenant ID
4. **Landing Page URL** — Configure in the offer: `https://<your-domain>/api/marketplace/azure/landing`
5. **Webhook URL** — Configure in the offer: `https://<your-domain>/api/marketplace/azure/webhook`

## Deployment

### Helm Values

```yaml
license:
  provider: "azure-marketplace"

azure:
  tenantID: "<azure-ad-tenant-id>"
  clientID: "<azure-ad-client-id>"
  clientSecret: "<azure-ad-client-secret>"   # Use K8s Secret instead
  planID: ""                                  # Optional default plan
```

Or use the overlay file:

```bash
helm install rbitr ./deploy/helm/rbitr \
  -f deploy/helm/rbitr/values-azure-marketplace.yaml \
  --set azure.tenantID=<tenant-id> \
  --set azure.clientID=<client-id>
```

**Important**: Store `clientSecret` in a Kubernetes Secret, not in values.yaml.

### AKS Workload Identity

```yaml
serviceAccount:
  create: true
  annotations:
    azure.workload.identity/client-id: "<client-id>"
```

### Environment Variables

| Variable                  | Required | Description                          |
|---------------------------|----------|--------------------------------------|
| `RBTR_LICENSE_PROVIDER`    | Yes     | Set to `azure-marketplace`           |
| `RBTR_AZURE_TENANT_ID`    | Yes     | Azure AD tenant ID                   |
| `RBTR_AZURE_CLIENT_ID`    | Yes     | Azure AD application (client) ID     |
| `RBTR_AZURE_CLIENT_SECRET` | Yes    | Azure AD client secret               |
| `RBTR_AZURE_PLAN_ID`      | No      | Default plan for new subscriptions   |

## Customer Activation Flow

1. Customer purchases via Azure Marketplace.
2. Azure redirects to the **landing page** with a marketplace token.
3. rbitr resolves the token via `ResolveToken` API.
4. rbitr activates the subscription via `ActivateSubscription` API.
5. Provider and Reporter are updated with the subscription details.

### Landing Page

```
GET /api/marketplace/azure/landing?token=<marketplace-purchase-token>
```

Returns `200 OK` with subscription details on success.

## Subscription Lifecycle

Azure sends webhook notifications for these events:

| Action         | Description                      | Handler Action                |
|----------------|----------------------------------|-------------------------------|
| `ChangePlan`   | Customer changed plan            | Update plan, refresh provider |
| `Suspend`      | Payment issue, subscription held | Mark license as invalid       |
| `Reinstate`    | Suspension lifted                | Re-activate, refresh provider |
| `Unsubscribe`  | Customer cancelled               | Mark license as invalid       |

### Webhook Endpoint

```
POST /api/marketplace/azure/webhook
Content-Type: application/json

{
  "action": "ChangePlan",
  "subscriptionId": "sub-123",
  "planId": "pro",
  "status": "InProgress"
}
```

The webhook always returns `200 OK` to acknowledge the event.

### Subscription Statuses

| Status                       | License Valid | Description                    |
|------------------------------|---------------|--------------------------------|
| `PendingFulfillmentStart`    | Yes (default) | Awaiting activation            |
| `Subscribed`                 | Yes           | Active subscription            |
| `Suspended`                  | No            | Payment issue                  |
| `Unsubscribed`               | No            | Cancelled                      |

## Usage Metering

rbitr reports usage to Azure via `BatchUsageEvent`. Records are aggregated by
hourly bucket and sent with:

- **Dimension**: `governed_actions`
- **ResourceID**: subscription ID
- **PlanID**: current plan

Reporting parameters:
- **Flush interval**: 1 hour
- **Max batch size**: 25 events
- **Early flush threshold**: 20 records

## Authentication

rbitr authenticates with Azure AD using the **client credentials flow**:

```
POST https://login.microsoftonline.com/{tenantID}/oauth2/v2.0/token
grant_type=client_credentials
client_id={clientID}
client_secret={clientSecret}
scope=20e940b3-4c77-4b0b-9a53-9e16a1b010a7/.default
```

Tokens are cached and refreshed automatically 5 minutes before expiry.

## Monitoring

- Log prefix: `azure marketplace:`
- Subscription refresh failures preserve last-known-good state.
- Metering failures re-buffer records for retry.
- Suspended subscriptions are logged as warnings.

## Troubleshooting

| Symptom                        | Cause                          | Fix                                         |
|--------------------------------|--------------------------------|---------------------------------------------|
| Landing page returns 401       | Invalid client credentials     | Verify tenant/client/secret in config       |
| Subscription not activating    | Missing ActivateSubscription   | Check landing page is called after purchase |
| License invalid after payment  | Subscription suspended         | Customer resolves payment, Reinstate fires  |
| Usage not reported             | Subscription ID not set        | Ensure landing page completes successfully  |
| Token acquisition fails        | Wrong scope or tenant          | Verify Azure AD app registration            |
