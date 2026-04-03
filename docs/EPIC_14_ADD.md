# EPIC 14 ADD — Cloud Marketplace Licensing & Metering

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** Interface Extraction & Self-Managed Adapter | DONE | 2026-03-27 |
| **2** AWS Marketplace Provider | DONE | 2026-03-30 |
| **3** GCP Marketplace Provider | DONE | 2026-04-02 |
| **4** Azure Marketplace Provider | DONE | 2026-04-03 |
| **5** Integration Testing & Documentation | TODO | — |

## Summary

Add cloud marketplace licensing providers (AWS Marketplace, GCP Marketplace, Azure Marketplace) to rbitr so the product can be sold through cloud marketplaces. This requires extracting a `LicenseProvider` interface from the current `*license.Validator`, extracting a `UsageReporter` interface from the current local-DB metering path, and implementing three marketplace-specific providers. The self-managed JWT path must continue working identically (zero behavioral change for existing deployments).

Date scoped: 2026-03-27

---

## Problem Statement

1. **Self-managed only.** The current licensing is an Ed25519-signed JWT file read from disk, validated in-memory, with usage metering stored locally in PostgreSQL. There is no way to integrate with cloud marketplace billing.
2. **No external metering.** Usage is tracked locally via `store.IncrementUsageMeter()` but never reported to external billing systems. Cloud marketplaces require the vendor to report consumption via their metering APIs.
3. **No marketplace activation flow.** Each marketplace has a specific activation handshake (AWS registration token, GCP Procurement approval, Azure landing page + webhook) that does not exist today.
4. **Tight coupling.** All license consumers depend directly on `*license.Validator`, making it impossible to swap in a different entitlement source without modifying every consumer.

---

## Current Architecture

### Concrete Coupling Points

Every license consumer depends on `*license.Validator` directly:

| Consumer | File | Methods Called |
|---|---|---|
| `admin.Dependencies` | `internal/api/admin/handlers.go` | `.Entitlements()`, `.Info()` |
| `public.Dependencies` | `internal/api/public/handlers.go` | `.Entitlements()` |
| `feature_gate.go` | `internal/api/admin/feature_gate.go` | `.Entitlements()` |
| `usage_quota.go` | `internal/api/public/usage_quota.go` | `.Entitlements()` |
| `usage_dashboard.go` | `internal/api/admin/usage_dashboard.go` | `.Entitlements()`, `.Info()` |
| `provisioning_limits.go` | `internal/api/admin/provisioning_limits.go` | `.Entitlements()` |
| `audit_visibility.go` | `internal/api/admin/audit_visibility.go` | `.Entitlements()` |
| `license_management.go` | `internal/api/admin/license_management.go` | `.Info()`, `.ValidateBytes()`, `.KeyPath()`, `.LoadAndValidate()` |
| `license_check.go` | `internal/api/public/license_check.go` | `.Entitlements()` |
| `main.go` | `cmd/gateway/main.go` | `NewValidator()`, `.LoadAndValidate()`, `NewWatcher()` |

### Usage Metering Path

Usage metering is purely local-DB via `store.StoreAPI.IncrementUsageMeter()`. The call site is `internal/api/public/usage_quota.go` — increments counter in DB per request. For marketplace providers, this same increment must also report to the external metering API (batched, not per-request).

### Marketplace Requirements by Cloud

| Concern | AWS Marketplace | GCP Marketplace | Azure Marketplace |
|---|---|---|---|
| **Entitlement check** | `aws-marketplace-metering` / License Manager API | Cloud Commerce Procurement API | SaaS Fulfillment API v2 |
| **Usage metering** | `MeterUsage` / `BatchMeterUsage` API | Usage-based pricing via Procurement | Marketplace Metering API |
| **Billing** | AWS handles invoicing | GCP handles invoicing | Azure handles invoicing |
| **Activation** | Customer subscribes → resolve registration token | Customer approves in Procurement | Landing page + webhook |

---

## Architecture Changes

### New Interfaces

#### LicenseProvider

```go
// LicenseProvider abstracts license entitlement resolution.
// Implementations: self-managed (JWT file), AWS Marketplace, GCP Marketplace, Azure Marketplace.
type LicenseProvider interface {
    // Info returns the current license metadata and entitlements.
    Info() LicenseInfo

    // Entitlements returns the current resolved entitlements (thread-safe).
    Entitlements() Entitlements

    // Start begins any background work (polling, cache refresh). Blocks until ctx is cancelled.
    Start(ctx context.Context)
}
```

#### UsageReporter

```go
// UsageReporter abstracts external usage reporting.
// Self-managed uses a no-op (local DB metering is sufficient).
// Marketplace providers batch and report to external metering APIs.
type UsageReporter interface {
    // RecordUsage records a single governed action for eventual external reporting.
    RecordUsage(ctx context.Context, tenantID, period string, quantity int64) error

    // Start begins the background flush loop. Blocks until ctx is cancelled.
    Start(ctx context.Context)
}
```

#### SelfManagedManager (optional, for upload/remove)

```go
// SelfManagedManager is an optional interface for providers that support
// manual license key upload and removal (self-managed only).
type SelfManagedManager interface {
    ValidateBytes(data []byte) (LicenseInfo, error)
    KeyPath() string
    LoadAndValidate()
}
```

The admin license upload/remove handlers check `provider, ok := d.LicenseProvider.(license.SelfManagedManager)` and return 501 when the provider is not self-managed. Marketplace licensing is managed externally (through the marketplace console).

### Provider Selection

At startup, `RBTR_LICENSE_PROVIDER` env var selects the implementation:

| Value | Provider | Reporter |
|---|---|---|
| `self-managed` (default) | `SelfManagedProvider` wrapping existing `*Validator` + `*Watcher` | `NoopReporter` |
| `aws-marketplace` | `aws.Provider` | `aws.Reporter` |
| `gcp-marketplace` | `gcp.Provider` | `gcp.Reporter` |
| `azure-marketplace` | `azure.Provider` | `azure.Reporter` |

### New Files

| File | Purpose |
|---|---|
| `internal/license/provider.go` | `LicenseProvider` + `SelfManagedManager` interface definitions |
| `internal/license/reporter.go` | `UsageReporter` interface definition |
| `internal/license/selfmanaged.go` | Self-managed provider wrapping existing `*Validator` + `*Watcher` |
| `internal/license/noop_reporter.go` | No-op `UsageReporter` for self-managed mode |
| `internal/license/factory.go` | Provider factory (`config → LicenseProvider + UsageReporter`) |
| `internal/license/aws/provider.go` | AWS Marketplace entitlement provider |
| `internal/license/aws/reporter.go` | AWS Marketplace usage reporter (`BatchMeterUsage`) |
| `internal/license/aws/activation.go` | AWS registration token resolution handler |
| `internal/license/gcp/provider.go` | GCP Marketplace entitlement provider |
| `internal/license/gcp/reporter.go` | GCP Marketplace usage reporter |
| `internal/license/gcp/activation.go` | GCP Procurement webhook handler |
| `internal/license/azure/provider.go` | Azure Marketplace entitlement provider |
| `internal/license/azure/reporter.go` | Azure Marketplace usage reporter |
| `internal/license/azure/activation.go` | Azure landing page + lifecycle webhook handler |
| `deploy/helm/rbitr/values-aws-marketplace.yaml` | AWS Marketplace Helm overlay |
| `deploy/helm/rbitr/values-gcp-marketplace.yaml` | GCP Marketplace Helm overlay |
| `deploy/helm/rbitr/values-azure-marketplace.yaml` | Azure Marketplace Helm overlay |

### Modified Files

| File | Change |
|---|---|
| `internal/config/config.go` | Add `LicenseProvider`, `AWSProductCode`, `AWSRegion`, `GCPProjectID`, `GCPServiceName`, `AzureTenantID`, `AzureClientID`, `AzureClientSecret`, `AzurePlanID` |
| `internal/api/admin/handlers.go` | `LicenseValidator *license.Validator` → `LicenseProvider license.LicenseProvider` |
| `internal/api/public/handlers.go` | `LicenseValidator *license.Validator` → `LicenseProvider license.LicenseProvider` |
| `internal/api/admin/feature_gate.go` | Use `d.LicenseProvider.Entitlements()` |
| `internal/api/admin/usage_dashboard.go` | Use `d.LicenseProvider` |
| `internal/api/admin/provisioning_limits.go` | Use `d.LicenseProvider` |
| `internal/api/admin/audit_visibility.go` | Use `d.LicenseProvider` |
| `internal/api/admin/license_management.go` | Type-assert to `SelfManagedManager`; return 501 for marketplace |
| `internal/api/public/usage_quota.go` | Use `d.LicenseProvider.Entitlements()`; call `UsageReporter.RecordUsage()` |
| `internal/api/public/license_check.go` | Use `d.LicenseProvider` |
| `cmd/gateway/main.go` | Use factory; start `provider.Start(ctx)` + `reporter.Start(ctx)` |
| `deploy/helm/rbitr/values.yaml` | Add `license.provider` field |
| `deploy/helm/rbitr/templates/configmap.yaml` | Add `RBTR_LICENSE_PROVIDER` |
| `deploy/helm/rbitr/templates/deployment-gateway.yaml` | Conditional marketplace env vars, IRSA annotations |

---

## Implementation Phases

### Phase 1: Interface Extraction & Self-Managed Adapter

**Goal**: Introduce interfaces, wrap existing code, change all consumers to use the interface. Self-managed behavior is byte-for-byte identical. Independently mergeable and deployable.

| Task | Size | Description | Dependencies |
|---|---|---|---|
| 1.1 | S | Define `LicenseProvider` interface in `internal/license/provider.go` | — |
| 1.2 | S | Define `UsageReporter` interface in `internal/license/reporter.go` | — |
| 1.3 | S | Define `SelfManagedManager` interface in `provider.go` | 1.1 |
| 1.4 | M | Implement `SelfManagedProvider` wrapping `*Validator` + `*Watcher` | 1.1, 1.3 |
| 1.5 | S | Implement `NoopReporter` | 1.2 |
| 1.6 | M | Implement provider factory (config → concrete provider) | 1.4, 1.5 |
| 1.7 | S | Add `LicenseProvider` config field (`RBTR_LICENSE_PROVIDER`, default `self-managed`) | — |
| 1.8 | M | Update `admin.Dependencies` field to `LicenseProvider license.LicenseProvider` | 1.1 |
| 1.9 | M | Update all admin consumers (`feature_gate`, `usage_dashboard`, `provisioning_limits`, `audit_visibility`) | 1.8 |
| 1.10 | M | Refactor `license_management.go` — status uses interface, upload/remove type-assert to `SelfManagedManager` | 1.3, 1.8 |
| 1.11 | M | Update `public.Dependencies` field to `LicenseProvider license.LicenseProvider` | 1.1 |
| 1.12 | S | Update all public consumers (`usage_quota`, `license_check`) | 1.11 |
| 1.13 | M | Update `main.go` wiring — factory call, pass to deps, start goroutines | 1.6, 1.8, 1.11 |
| 1.14 | M | Write/update tests — `SelfManagedProvider`, factory, handler tests with mock `LicenseProvider` | 1.4, 1.6 |
| 1.15 | S | Update Helm configmap — add `RBTR_LICENSE_PROVIDER` | 1.7 |
| 1.16 | S | Update Helm `values.yaml` — add `license.provider: "self-managed"` | — |

**Exit criteria**: All existing tests pass. No code in `internal/api/` references `*license.Validator` directly. `RBTR_LICENSE_PROVIDER=self-managed` (default) is behaviorally identical.

---

### Phase 2: AWS Marketplace Provider

**Goal**: Full AWS Marketplace entitlement + metering integration. Can start immediately after Phase 1 merges.

| Task | Size | Description | Dependencies |
|---|---|---|---|
| 2.1 | S | Add AWS config fields (`RBTR_AWS_PRODUCT_CODE`, `RBTR_AWS_REGION`) | Phase 1 |
| 2.2 | L | Implement AWS entitlement provider — `ResolveCustomer`, `GetEntitlements`, cache with 5m TTL, map dimensions to `Entitlements` | 1.1 |
| 2.3 | L | Implement AWS usage reporter — buffer in memory, flush via `BatchMeterUsage` hourly, deduplication via `UsageAllocations`, crash recovery via local DB | 1.2 |
| 2.4 | M | Implement registration token resolver — `POST /api/marketplace/aws/resolve-token`, calls `ResolveCustomer`, stores customer ID | 2.2 |
| 2.5 | S | Register AWS activation route (when provider is `aws-marketplace`) | 2.4 |
| 2.6 | S | Update factory — `case "aws-marketplace"` | 2.2, 2.3 |
| 2.7 | L | Write tests — mocked AWS SDK clients, entitlement mapping, batch flush timing, error recovery | 2.2, 2.3 |
| 2.8 | M | AWS Helm values overlay (`values-aws-marketplace.yaml`) — provider, product code, IRSA annotation | Phase 1 Helm |
| 2.9 | M | Update Helm deployment template — conditional AWS env vars, service account annotations | 2.8 |
| 2.10 | M | Update CloudFormation template — IAM policy for `aws-marketplace:*`, `entitlement.marketplace:GetEntitlements`, env vars | 2.1 |

**AWS IAM permissions required**:
- `aws-marketplace:ResolveCustomer`
- `aws-marketplace:BatchMeterUsage`
- `aws-marketplace:MeterUsage`
- `entitlement.marketplace:GetEntitlements`

---

### Phase 3: GCP Marketplace Provider

**Goal**: Full GCP Marketplace entitlement + metering integration. Can start immediately after Phase 1 merges. Independent of Phase 2.

| Task | Size | Description | Dependencies |
|---|---|---|---|
| 3.1 | S | Add GCP config fields (`RBTR_GCP_PROJECT_ID`, `RBTR_GCP_SERVICE_NAME`) | Phase 1 |
| 3.2 | L | Implement GCP entitlement provider — Cloud Commerce Procurement API, map plans to `Entitlements`, cache with TTL | 1.1 |
| 3.3 | M | Implement GCP usage reporter — Service Control API or Procurement usage API (depends on pricing model), buffer + batch | 1.2 |
| 3.4 | M | Implement Procurement webhook handler — `POST /api/marketplace/gcp/webhook`, handles Pub/Sub push for entitlement state changes, verifies JWT | 3.2 |
| 3.5 | S | Update factory — `case "gcp-marketplace"` | 3.2, 3.3 |
| 3.6 | L | Write tests — mocked GCP API clients, state machine (ACTIVE/CANCELLED), Pub/Sub verification | 3.2, 3.3 |
| 3.7 | M | GCP Helm values overlay (`values-gcp-marketplace.yaml`) — provider, project ID, Workload Identity annotation | Phase 1 Helm |

**GCP IAM roles required**:
- `roles/commerceoffercatalog.viewer` (entitlement reads)
- `roles/servicemanagement.serviceController` (usage reporting)

---

### Phase 4: Azure Marketplace Provider

**Goal**: Full Azure Marketplace entitlement + metering integration. Can start immediately after Phase 1 merges. Independent of Phases 2-3.

| Task | Size | Description | Dependencies |
|---|---|---|---|
| 4.1 | S | Add Azure config fields (`RBTR_AZURE_TENANT_ID`, `RBTR_AZURE_CLIENT_ID`, `RBTR_AZURE_CLIENT_SECRET`, `RBTR_AZURE_PLAN_ID`) | Phase 1 |
| 4.2 | L | Implement Azure entitlement provider — SaaS Fulfillment API v2, AAD token acquisition, subscription lifecycle (PendingFulfillmentStart → Subscribed → Suspended → Unsubscribed), map plans to `Entitlements` | 1.1 |
| 4.3 | M | Implement Azure usage reporter — Marketplace Metering API (`usageEvent`, `batchUsageEvent`), UTC hour boundaries, buffer + batch | 1.2 |
| 4.4 | M | Implement landing page + lifecycle webhook — `GET /api/marketplace/azure/landing` (resolves marketplace token), `POST /api/marketplace/azure/webhook` (ChangePlan, Suspend, Reinstate, Unsubscribe), must respond within 10s | 4.2 |
| 4.5 | S | Update factory — `case "azure-marketplace"` | 4.2, 4.3 |
| 4.6 | L | Write tests — mocked Azure API clients, subscription state machine, hour-boundary batching, webhook idempotency | 4.2, 4.3 |
| 4.7 | M | Azure Helm values overlay (`values-azure-marketplace.yaml`) — provider, tenant/client IDs, Workload Identity annotations | Phase 1 Helm |

---

### Phase 5: Integration Testing & Documentation

**Goal**: End-to-end validation of each provider, documentation for marketplace listing submissions.

| Task | Size | Description | Dependencies |
|---|---|---|---|
| 5.1 | L | Integration test harness — localstack (AWS), emulated GCP APIs, mock Azure endpoints. Full lifecycle: activation → entitlement → metering → deactivation | Phases 2-4 |
| 5.2 | M | E2E test: self-managed backward compatibility — full startup-to-request with `RBTR_LICENSE_PROVIDER=self-managed` | Phase 1 |
| 5.3 | M | Marketplace listing documentation (`docs/marketplace/aws-listing.md`, `gcp-listing.md`, `azure-listing.md`) — listing requirements, IAM policies, deployment instructions, dimension mappings | Phases 2-4 |
| 5.4 | S | Update README and architecture docs | Phase 1 |

---

## Dependency Graph

```
Phase 1 (Interface Extraction)
  │
  ├──→ Phase 2 (AWS)       ─┐
  │                          │
  ├──→ Phase 3 (GCP)       ─┼──→ Phase 5 (Integration + Docs)
  │                          │
  └──→ Phase 4 (Azure)     ─┘
```

Phases 2, 3, and 4 are fully independent and can be developed in parallel.

---

## Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| **Breaking self-managed behavior** during interface extraction | HIGH | Phase 1 is purely mechanical (rename + delegation). Existing test suite must pass with zero assertion changes. |
| **AWS API throttling** on `GetEntitlements` | MEDIUM | Cache entitlements for 5 min. Exponential backoff. On persistent failure, use last-known-good entitlements (never downgrade to free on transient errors). |
| **Usage metering data loss** on crash | HIGH | Persist pending meter records to local DB (`store.StoreAPI`) before flushing to external API. On startup, drain unflushed records. |
| **Clock skew** between gateway and marketplace billing | MEDIUM | Use NTP-synced time. Azure: always use UTC hour boundaries. AWS: include `UsageAllocations` with explicit timestamps. |
| **IAM/credential misconfiguration** at deployment | HIGH | Validate cloud credentials at startup (test API call). Fail fast with clear error if invalid. |
| **Marketplace webhook replay/out-of-order** | MEDIUM | Idempotent webhook handlers. Store last-processed event timestamp. Use marketplace sequence numbers where available. |
| **Large binary size** from three cloud SDKs | LOW | Use Go build tags (`//go:build aws_marketplace`) to compile only the needed provider. Default build includes only self-managed. |

---

## Testing Strategy

### Unit Tests (every phase)

| Component | Focus |
|---|---|
| `SelfManagedProvider` | Delegation to `Validator`, nil safety, `Start` context cancellation |
| `NoopReporter` | `RecordUsage` is no-op, `Start` blocks then returns |
| Factory | Correct provider for each config value, error on unknown |
| AWS Provider | Entitlement mapping, cache refresh, error fallback, throttle handling |
| AWS Reporter | Batch accumulation, flush timing, dedup, error recovery, shutdown drain |
| GCP Provider | Procurement API mapping, state machine, Pub/Sub verification |
| Azure Provider | SaaS API token flow, subscription states, plan mapping |
| Azure Reporter | Hour-boundary batching, metering API calls, idempotency |
| Updated handlers | All admin/public handlers with mock `LicenseProvider` |

### Integration Tests

- Self-managed: startup with license file, hot-reload, file removal fallback
- AWS: registration token → entitlement check → meter usage (localstack)
- GCP: Procurement webhook → entitlement active → usage report (emulated)
- Azure: landing page resolve → subscription active → meter usage (mock server)

---

## Estimated Effort

| Phase | Complexity | Est. Days | Parallelizable |
|---|---|---|---|
| Phase 1: Interface Extraction | M | 3-4 | No (must be first) |
| Phase 2: AWS Marketplace | L | 5-7 | Yes (after Phase 1) |
| Phase 3: GCP Marketplace | L | 5-7 | Yes (after Phase 1) |
| Phase 4: Azure Marketplace | L | 5-7 | Yes (after Phase 1) |
| Phase 5: Integration + Docs | M | 3-4 | After 2/3/4 |
| **Total (sequential)** | | **21-29** | |
| **Total (parallel 2-4)** | | **11-15** | |

---

## Success Criteria

- [ ] `RBTR_LICENSE_PROVIDER=self-managed` (default) behaves identically to current code — all existing tests pass
- [ ] `RBTR_LICENSE_PROVIDER=aws-marketplace` resolves customer token, checks entitlements via AWS API, reports usage via `BatchMeterUsage`
- [ ] `RBTR_LICENSE_PROVIDER=gcp-marketplace` checks entitlements via Procurement API, handles Pub/Sub lifecycle events
- [ ] `RBTR_LICENSE_PROVIDER=azure-marketplace` resolves subscription via SaaS Fulfillment API, reports usage via Metering API, handles lifecycle webhooks
- [ ] No code in `internal/api/` directly references `*license.Validator` (all go through `LicenseProvider` interface)
- [ ] Usage metering data is never lost on crash (persisted locally before external flush)
- [ ] Helm overlays for each marketplace deploy successfully on respective cloud Kubernetes services
- [ ] CloudFormation template deploys with correct IAM permissions for AWS Marketplace metering
- [ ] Test coverage >= 80% for all new code
- [ ] License upload/remove endpoints gracefully return 501 for marketplace providers
