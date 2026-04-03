# GCP Marketplace Listing Guide

## Overview

rbitr integrates with GCP Marketplace via the **Cloud Commerce Partner Procurement
API** (for entitlement management) and the **Service Control API v2** (for usage
reporting). Entitlement lifecycle events are received via **Pub/Sub push webhooks**.

## Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌──────────────┐
│  GCP Marketplace │────▶│  Procurement API     │────▶│   rbitr      │
│  (customer sub)  │     │  (ListEntitlements)  │     │   Provider   │
└─────────────────┘     └──────────────────────┘     └──────────────┘
        │                                                   │
        │ Pub/Sub push     ┌──────────────────────┐         │
        └─────────────────▶│  Webhook Handler     │         │
                           │  POST /api/mp/gcp/wh │         │
                           └──────────────────────┘         │
                           ┌──────────────────────┐         │
                           │  Service Control v2  │◀────────┘
                           │  (Report)            │  Reporter
                           └──────────────────────┘
```

## Plan Tiers

| Plan         | MaxTenants | Agents/Tenant | MonthlyActions | Approval | Evidence |
|--------------|------------|---------------|----------------|----------|----------|
| `starter`    | 5          | 10            | 10,000         | No       | No       |
| `pro`        | 25         | 50            | 100,000        | Yes      | Yes      |
| `enterprise` | Unlimited  | Unlimited     | Unlimited      | Yes      | Yes      |

Enterprise plans use `PaidTierDefaults()` which sets numeric limits to `-1` (unlimited).

Custom entitlement properties can override plan defaults via the `InputProperties`
field on GCP entitlements (JSON with keys like `max_tenants`, `monthly_actions`, etc.).

## Prerequisites

1. **Partner Portal** — Register at [GCP Partner Portal](https://cloud.google.com/marketplace/docs/partners).
2. **Product Listing** — Create a SaaS product with the plan tiers above.
3. **Pub/Sub Topic** — Configure a push subscription for entitlement lifecycle events.
4. **Service Account** — The GKE workload identity needs:
   - `cloudcommerceprocurement.entitlements.list`
   - `cloudcommerceprocurement.entitlements.approve`
   - `servicecontrol.services.report`

## Deployment

### Helm Values

```yaml
license:
  provider: "gcp-marketplace"

gcp:
  projectID: "my-project-id"     # GCP project hosting the product
  serviceName: "rbitr-service"   # Product's external name in Procurement API
```

Or use the overlay file:

```bash
helm install rbitr ./deploy/helm/rbitr \
  -f deploy/helm/rbitr/values-gcp-marketplace.yaml \
  --set gcp.projectID=my-project-id \
  --set gcp.serviceName=rbitr-service
```

### Workload Identity

```yaml
serviceAccount:
  create: true
  annotations:
    iam.gke.io/gcp-service-account: "rbitr-marketplace@my-project.iam.gserviceaccount.com"
```

### Environment Variables

| Variable                | Required | Description                              |
|-------------------------|----------|------------------------------------------|
| `RBTR_LICENSE_PROVIDER`  | Yes     | Set to `gcp-marketplace`                 |
| `RBTR_GCP_PROJECT_ID`   | Yes     | GCP project ID (used as provider ID)     |
| `RBTR_GCP_SERVICE_NAME`  | Yes     | Product external name in Procurement API |

## Entitlement Lifecycle

GCP Marketplace sends Pub/Sub notifications for these events:

| Event                                    | Handler Action                          |
|------------------------------------------|-----------------------------------------|
| `ENTITLEMENT_CREATION_REQUESTED`         | Auto-approve via `ApproveEntitlement`   |
| `ENTITLEMENT_ACTIVE`                     | Refresh entitlements from API           |
| `ENTITLEMENT_CANCELLED`                  | Refresh (will find no active entitlement) |
| `ENTITLEMENT_PENDING_CANCELLATION`       | Refresh (still active until billing end) |
| `ENTITLEMENT_PLAN_CHANGE_REQUESTED`      | Auto-approve plan change                |
| `ENTITLEMENT_PLAN_CHANGED`               | Refresh to pick up new plan             |

### Webhook Endpoint

```
POST /api/marketplace/gcp/webhook
Content-Type: application/json

{
  "message": {
    "data": "<base64-encoded-json>"
  }
}
```

The webhook always returns `200 OK` to acknowledge the Pub/Sub message.

## Usage Reporting

rbitr reports usage to GCP via Service Control API v2. Records are aggregated by
hourly bucket and sent as `AttributeContext` operations with:

- **Operation**: `rbitr.governed_action`
- **Service**: configured service name
- **Quantity**: `Request.Size` field

Reporting parameters:
- **Flush interval**: 1 hour
- **Max batch size**: 100 operations
- **Early flush threshold**: 80 records

## Monitoring

- Log prefix: `gcp marketplace:`
- Entitlement refresh failures preserve last-known-good state.
- Service Control Report failures re-buffer records for retry.

## Troubleshooting

| Symptom                        | Cause                          | Fix                                        |
|--------------------------------|--------------------------------|--------------------------------------------|
| Defaults instead of plan       | Wrong `serviceName`            | Verify `RBTR_GCP_SERVICE_NAME` matches listing |
| Webhook events not received    | Pub/Sub misconfigured          | Check push subscription URL and IAM        |
| Auto-approve not working       | Missing approve permission     | Grant `cloudcommerceprocurement.entitlements.approve` |
| Usage not reported             | Service Control API disabled   | Enable Service Control API in GCP console  |
