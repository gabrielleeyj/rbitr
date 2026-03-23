# EPIC 13 — Freemium Model & License Key Enforcement

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** License Key Validation & Entitlement Model | **TODO** | — |
| **2** Usage Metering & Quota Enforcement | **TODO** | — |
| **3** Tenant Provisioning Limits (Free Tier) | **TODO** | — |
| **4** Feature Gating (API + UI) | **TODO** | — |
| **5** License Management UI | **TODO** | — |
| **6** Admin Dashboard & Usage Visibility | **TODO** | — |

## Summary

Epic 13 introduces a freemium model for rbitr as a self-hosted product. Customers install rbitr on their own infrastructure and operate in free tier by default. Purchasing a license key unlocks paid-tier features. The license key is a cryptographically signed token (Ed25519) that encodes tier, entitlements, and expiry — validated entirely offline with no phone-home requirement.

Date scoped: 2026-03-23

---

## Problem Statement

rbitr currently treats all installations equally — every feature is available with no usage limits. As a self-hosted product, this makes it impossible to:

1. **Offer a free tier:** Users cannot evaluate rbitr with a constrained but functional experience before purchasing.
2. **Control resource consumption:** A free installation can generate unbounded audit logs, keys, and governed actions with no metering.
3. **Monetize advanced features:** Approval workflows, multi-agent orchestration, integrations, and evidence export are available to everyone without a license.
4. **Enforce license expiry:** There is no mechanism to time-bound paid access or degrade gracefully when a license expires.

---

## Tier Definitions

### Free Tier (No License Key)

| Dimension | Limit |
|-----------|-------|
| Tenants | 1 |
| Agents per tenant | 1 |
| Active tenant keys | 1 |
| Governed actions / month | 10,000 |
| Audit log retention | 7 days |
| Approval workflows | Not available |
| Evidence export | Not available |
| Integrations (Slack, Jira, etc.) | Not available |
| Notification channels | Email only |
| Policy engine | Default policy only (no custom OPA) |

### Paid Tier (Valid License Key)

| Dimension | Limit |
|-----------|-------|
| Tenants | Unlimited (multi-tenant) |
| Agents per tenant | Unlimited (multi-agent) |
| Active tenant keys | Unlimited |
| Governed actions / month | Unlimited |
| Audit log retention | 90 days (configurable up to 1 year) |
| Approval workflows | Full access |
| Evidence export | Full access (PDF, CSV, JSON) |
| Integrations | All (Slack, Jira, ServiceNow, Linear, Telegram, WhatsApp) |
| Notification channels | All channels |
| Policy engine | Custom OPA policies, base policy override |

---

## License Key Design

### Format

The license key is a Base64-encoded signed JWT (EdDSA / Ed25519) containing:

```json
{
  "iss": "rbitr",
  "sub": "customer-org-name",
  "iat": 1711152000,
  "exp": 1742688000,
  "key_version": 1,
  "tier": "paid",
  "entitlements": {
    "max_tenants": -1,
    "max_agents_per_tenant": -1,
    "max_active_keys": -1,
    "monthly_action_limit": -1,
    "audit_retention_days": 365,
    "approval_workflows": true,
    "evidence_export": true,
    "integrations": true,
    "custom_policies": true
  },
  "licensee": {
    "name": "Acme Corp",
    "email": "admin@acme.com"
  }
}
```

(`-1` = unlimited)

### Validation Flow

1. Gateway starts → reads `license.key` file from config directory (or `RBITR_LICENSE_KEY` env var)
2. If no key found → free tier entitlements applied
3. If key found → verify Ed25519 signature using embedded public key
4. If signature invalid → reject key, log warning, fall back to free tier
5. If signature valid → check `exp` claim against current time
6. If expired → log warning, fall back to free tier (graceful degradation)
7. If valid → cache resolved entitlements in memory, re-check periodically (every hour)

### Key Generation (Internal Tooling)

A CLI tool (`cmd/license-gen`) generates signed license keys:

```bash
rbitr-license-gen \
  --private-key /path/to/private.pem \
  --licensee "Acme Corp" \
  --email "admin@acme.com" \
  --tier paid \
  --expires 2027-03-23
```

The private key is never distributed. Only the public key is embedded in the gateway binary.

### Security Properties

- **Offline validation** — no network call required, works in air-gapped environments
- **Tamper-proof** — Ed25519 signature; modifying any claim invalidates the key
- **Non-transferable** — licensee name/email embedded for audit trail
- **Time-bounded** — expiry enforced at startup + periodic re-check
- **Graceful degradation** — expired/invalid key → free tier (never hard lockout)

### Version Compatibility

- License keys include a **`key_version`** field (integer, starting at `1`) that identifies the key format version
- The gateway maintains a **`min_supported_key_version`** constant that defines the oldest key version it will accept
- **Backwards compatibility:** Each new app release supports all key versions from `min_supported_key_version` through the current version. Older keys remain valid until they expire naturally.
- **Deprecation path:** Future app releases can bump `min_supported_key_version` to stop accepting very old key formats, giving customers a migration window (key must expire first or customer regenerates)
- **Security benefit:** If a vulnerability is found in an older key format (e.g., weaker claims, missing fields), a new app version can refuse those keys by raising the minimum version
- Entitlement resolution uses **merge-over-defaults**: key claims are merged on top of tier defaults, so if a newer key version introduces entitlement fields that an older key doesn't contain, it falls back to the tier default rather than failing
- Public key rotation (if ever needed) would use a **key ID (`kid`)** header in the JWT, allowing the gateway to accept both old and new keys during a transition period

#### Key Version Validation Flow

```
1. Parse JWT → extract `key_version` claim
2. If `key_version` < `min_supported_key_version` →
     reject with: "License key version too old. Please contact support for a new key."
3. If `key_version` > `current_key_version` →
     reject with: "License key requires a newer version of rbitr. Please upgrade."
4. Otherwise → proceed with signature verification and entitlement resolution
```

#### Version History (maintained in code)

```go
const (
    CurrentKeyVersion      = 1  // latest key format
    MinSupportedKeyVersion = 1  // oldest accepted key format
)
```

---

## Phase 1 — License Key Validation & Entitlement Model

### Problem

There is no concept of a license or tier in the system. All features are implicitly available.

### Solution

Add a license validation module that reads, verifies, and caches the license key, plus a tier/entitlement system that all other components query.

#### Components

1. **`internal/license/validator.go`** — Ed25519 signature verification, JWT parsing, expiry checks
2. **`internal/license/entitlements.go`** — Resolved entitlement struct with helper methods (`HasFeature`, `MaxTenants`, etc.)
3. **`internal/license/defaults.go`** — Free-tier and paid-tier default entitlements
4. **`internal/license/watcher.go`** — File watcher that detects license.key changes and re-validates (hot reload without restart)

#### Entitlements Model

```go
type Entitlements struct {
    Tier                 string `json:"tier"`
    MaxTenants           int    `json:"max_tenants"`           // -1 = unlimited
    MaxAgentsPerTenant   int    `json:"max_agents_per_tenant"` // -1 = unlimited
    MaxActiveKeys        int    `json:"max_active_keys"`       // -1 = unlimited
    MonthlyActionLimit   int64  `json:"monthly_action_limit"`  // -1 = unlimited
    AuditRetentionDays   int    `json:"audit_retention_days"`
    ApprovalWorkflows    bool   `json:"approval_workflows"`
    EvidenceExport       bool   `json:"evidence_export"`
    Integrations         bool   `json:"integrations"`
    CustomPolicies       bool   `json:"custom_policies"`
}

type LicenseInfo struct {
    Valid        bool         `json:"valid"`
    Tier         string       `json:"tier"`
    Licensee     string       `json:"licensee"`
    Email        string       `json:"email"`
    ExpiresAt    time.Time    `json:"expires_at"`
    Entitlements Entitlements `json:"entitlements"`
}
```

#### Migration: `00031_license.sql`

```sql
-- Store license key metadata (not the key itself) for audit trail
CREATE TABLE rbitr.license_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier         TEXT NOT NULL,
    licensee     TEXT NOT NULL,
    email        TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    fingerprint  TEXT NOT NULL  -- SHA-256 of the key for dedup
);
```

### Tasks

- [ ] Embed Ed25519 public key in gateway binary via `embed` directive
- [ ] Implement JWT parsing and Ed25519 signature verification
- [ ] Implement key version validation (`min_supported_key_version` through `current_key_version`)
- [ ] Implement expiry checking with graceful fallback to free tier
- [ ] Define free-tier and paid-tier default entitlements
- [ ] Implement `Entitlements` struct with `HasFeature`, `IsUnlimited` helpers
- [ ] Implement file watcher for hot-reload of license.key changes
- [ ] Create migration `00031_license.sql` for license audit trail
- [ ] Build `cmd/license-gen` CLI tool for internal key generation
- [ ] Write unit tests for validation (valid key, expired key, tampered key, missing key, unsupported version, future version)
- [ ] Write unit tests for entitlement resolution

---

## Phase 2 — Usage Metering & Quota Enforcement

### Problem

There is no tracking of how many governed actions have been consumed in a billing period. Without metering, the free-tier 10k action limit cannot be enforced.

### Solution

Introduce a `usage_meters` table that tracks monthly action counts, and a middleware that increments the counter on every governed action and rejects requests when the quota is exceeded.

#### Migration: `00032_usage_meters.sql`

```sql
CREATE TABLE rbitr.usage_meters (
    tenant_id    UUID NOT NULL REFERENCES rbitr.tenants(id),
    period       TEXT NOT NULL,  -- 'YYYY-MM' format
    action_count BIGINT NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, period)
);
```

#### Metering Flow

1. Request arrives at gateway → authentication resolves tenant
2. Metering middleware checks entitlements — if `monthly_action_limit == -1` (unlimited), skip metering
3. Load current period meter for tenant
4. If `action_count >= monthly_action_limit` → reject with `429 Too Many Requests`
5. Otherwise → increment counter atomically
6. Request proceeds to policy evaluation

#### Performance Considerations

- Use an in-memory counter with periodic flush (every N actions or every M seconds) to avoid per-request DB writes
- For free tier (10k/month), direct Postgres `UPDATE` is sufficient — ~333 actions/day is low volume
- Paid tier skips metering entirely (unlimited)

### Tasks

- [ ] Create migration `00032_usage_meters.sql`
- [ ] Implement `UsageMeterStore` with `Increment` and `GetCurrentUsage` methods
- [ ] Implement metering middleware in `internal/api/middleware/metering.go`
- [ ] Add in-memory buffered counter with configurable flush interval
- [ ] Return `429` with upgrade prompt when quota exceeded
- [ ] Write unit tests for counter logic and quota enforcement
- [ ] Write integration test for metering middleware

---

## Phase 3 — Tenant Provisioning Limits (Free Tier)

### Problem

Free-tier installations should only be able to create 1 tenant, 1 agent, and 1 active key. Currently, tenant/agent/key creation has no limit validation.

### Solution

Add pre-creation checks in the provisioning handlers that validate against the current entitlements before allowing creation.

#### Enforcement Points

| Resource | Check | Error |
|----------|-------|-------|
| Tenant creation | `COUNT(tenants) < max_tenants` | `403 Tenant limit reached. Upload a license key to create more tenants.` |
| Agent registration | `COUNT(agents WHERE tenant_id = ?) < max_agents_per_tenant` | `403 Agent limit reached. Upload a license key to register more agents.` |
| Key generation | `COUNT(active keys WHERE tenant_id = ?) < max_active_keys` | `403 Active key limit reached. Revoke an existing key or upload a license key.` |

### Tasks

- [ ] Add `CountTenants`, `CountAgentsByTenant`, `CountActiveKeysByTenant` store methods
- [ ] Add provisioning limit checks in tenant creation handler
- [ ] Add provisioning limit checks in agent registration handler
- [ ] Add provisioning limit checks in key generation handler
- [ ] Return clear error messages with license upload prompts
- [ ] Write unit tests for each limit check
- [ ] Write integration test for provisioning flow

---

## Phase 4 — Feature Gating (API + UI)

### Problem

Advanced features must be unavailable on free tier. On the API side, gated endpoints must return clear errors. On the UI side, gated features must be visually disabled with an "Upgrade to unlock" CTA on hover.

### Solution

#### API: Feature Gate Middleware

```go
func FeatureGate(feature string) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            ent := license.GetEntitlements()
            if !ent.HasFeature(feature) {
                return c.JSON(http.StatusForbidden, map[string]string{
                    "error":   "feature_not_available",
                    "message": fmt.Sprintf("%s requires a paid license. Upload a license key to unlock.", feature),
                    "feature": feature,
                })
            }
            return next(c)
        }
    }
}
```

#### Gated Features

| Feature | Gated Endpoints | Entitlement Key |
|---------|----------------|-----------------|
| Approval workflows | `POST /api/v1/approvals/*` | `approval_workflows` |
| Evidence export | `GET /api/v1/audit/export` | `evidence_export` |
| Integrations config | `POST /api/v1/integrations/*` | `integrations` |
| Custom OPA policies | `PUT /api/v1/tenants/:id/policy` | `custom_policies` |

#### UI: Entitlements API + Disabled State

The UI fetches entitlements on load via `GET /api/v1/license/entitlements` and uses the response to control feature visibility:

```json
{
  "tier": "free",
  "features": {
    "approval_workflows": false,
    "evidence_export": false,
    "integrations": false,
    "custom_policies": false
  }
}
```

##### UI Behavior for Gated Features

- **Disabled state:** Gated nav items, buttons, and cards render with reduced opacity (`opacity: 0.5`) and `pointer-events: none` on the action element
- **Lock icon:** A small lock icon overlaid on the feature card/button
- **Hover tooltip CTA:** On hover over the disabled element's container, show a tooltip: _"Upgrade to unlock — upload a license key in Settings > License"_
- **Settings redirect:** Tooltip includes a link to the license management page

##### Example: Gated Sidebar Nav Item

```
[lock icon] Approval Workflows     (dimmed)
            ┌─────────────────────────────────┐
            │ Upgrade to unlock                │
            │ Upload a license key in          │
            │ Settings > License               │
            └─────────────────────────────────┘
```

### Tasks

- [ ] Implement `FeatureGate` middleware in `internal/api/middleware/feature_gate.go`
- [ ] Add `GET /api/v1/license/entitlements` endpoint
- [ ] Register feature gates on all gated route groups
- [ ] Implement `LockedFeature` UI wrapper component with disabled state + tooltip CTA
- [ ] Apply `LockedFeature` to approval workflows nav/page
- [ ] Apply `LockedFeature` to evidence export button
- [ ] Apply `LockedFeature` to integrations nav/page
- [ ] Apply `LockedFeature` to custom policy editor
- [ ] Write unit tests for feature gate middleware
- [ ] Write unit tests for `LockedFeature` component
- [ ] Write integration test verifying free-tier cannot access gated endpoints

---

## Phase 5 — License Management UI

### Problem

Users need a way to upload and manage their license key from the admin UI without editing config files or environment variables.

### Solution

Add a License page under Settings where users can upload a `license.key` file, view current license status, and see when it expires.

#### License Settings Page

```
Settings > License
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Current Plan: Free Tier
┌─────────────────────────────────────────────┐
│  Upload License Key                         │
│                                             │
│  [  Drop license.key file here  ]           │
│  [  or click to browse           ]          │
│                                             │
└─────────────────────────────────────────────┘

── After upload: ──────────────────────────────

Current Plan: Paid
Licensed to: Acme Corp (admin@acme.com)
Expires: 2027-03-23 (365 days remaining)
Status: ● Active

[Remove License]
```

#### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/license` | Current license status (tier, licensee, expiry, valid) |
| `POST` | `/api/v1/license` | Upload license key file (multipart form or raw body) |
| `DELETE` | `/api/v1/license` | Remove license key, revert to free tier |

#### Upload Flow

1. User uploads `license.key` file via UI
2. Backend validates signature and expiry
3. If valid → write to config directory, update in-memory entitlements, log to `license_history`
4. If invalid → return error with specific reason (expired, tampered, malformed)
5. UI refreshes to show new license status and unlocked features

### Tasks

- [ ] Implement `POST /api/v1/license` upload handler with validation
- [ ] Implement `GET /api/v1/license` status endpoint
- [ ] Implement `DELETE /api/v1/license` removal endpoint
- [ ] Write license key to gateway config directory on upload
- [ ] Trigger hot-reload of entitlements after upload
- [ ] Build License Settings page UI with file drop zone
- [ ] Show license status (tier, licensee, expiry, days remaining)
- [ ] Show validation errors on failed upload (expired, tampered, malformed)
- [ ] Add [Remove License] button with confirmation dialog
- [ ] Write unit tests for upload/validate/remove handlers
- [ ] Write integration test for full upload flow

---

## Phase 6 — Admin Dashboard & Usage Visibility

### Problem

Users need visibility into their current usage, remaining quota, and tier status so that free-tier limits feel transparent rather than arbitrary.

### Solution

Add usage dashboard endpoints and a UI component showing current consumption, limits, and upgrade prompts.

#### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/usage` | Current period usage summary |
| `GET` | `/api/v1/usage/history` | Historical usage (last 6 months) |

#### Usage Response Schema

```json
{
  "tier": "free",
  "period": "2026-03",
  "usage": {
    "governed_actions": { "used": 4521, "limit": 10000, "pct": 45.2 },
    "tenants": { "used": 1, "limit": 1 },
    "agents": { "used": 1, "limit": 1 },
    "active_keys": { "used": 1, "limit": 1 }
  },
  "features": {
    "approval_workflows": false,
    "evidence_export": false,
    "integrations": false,
    "custom_policies": false
  },
  "audit_retention_days": 7,
  "license": {
    "valid": false,
    "tier": "free",
    "upload_url": "/settings/license"
  }
}
```

#### Usage Dashboard UI

- Progress bars for governed actions (with color: green < 60%, yellow 60-80%, red > 80%)
- Resource counts (tenants, agents, keys) with limits
- Feature availability list with lock icons for gated features
- Warning banner at 80% and 95% quota usage
- CTA: "Upload a license key to remove limits" linking to Settings > License

### Tasks

- [ ] Implement usage summary endpoint
- [ ] Implement usage history endpoint
- [ ] Add usage response models
- [ ] Build usage dashboard UI with progress bars and quota warnings
- [ ] Add warning banner at 80% and 95% usage thresholds
- [ ] Add CTA linking to license upload page
- [ ] Write unit tests for usage calculations
- [ ] Write integration test for usage endpoints

---

## Audit Log Retention Enforcement

As part of the free-tier constraints, audit log retention must be enforced at 7 days. This requires a background job that prunes audit records older than the entitlement's `audit_retention_days`.

### Implementation

- Add a periodic cleanup job in `internal/audit/retention.go`
- Job runs daily, deletes audit records where `created_at < now() - retention_days`
- Retention period resolved from current entitlements (7 days free, 90+ days paid)
- Job must be idempotent and use batch deletes to avoid lock contention

### Tasks

- [ ] Implement retention cleanup job
- [ ] Add retention period resolution from entitlements
- [ ] Add configuration for cleanup schedule
- [ ] Write unit tests for retention logic
- [ ] Write integration test for cleanup job

---

## Dependencies

| Dependency | Required By | Notes |
|------------|-------------|-------|
| Ed25519 (`crypto/ed25519`) | Phase 1 | Stdlib, no external dependency |
| JWT library (`github.com/golang-jwt/jwt/v5`) | Phase 1 | License token parsing |
| `fsnotify` (`github.com/fsnotify/fsnotify`) | Phase 1 | License file hot-reload watcher |
| Existing tenant key system | Phase 3 | Key count validation |
| Existing audit log table | Retention | Must support date-based deletion |

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Users bypass license by patching binary | Revenue loss | Acceptable risk for self-hosted; legal protection via EULA. Obfuscation not worth the complexity. |
| Metering adds latency to every request | Performance degradation | In-memory buffered counter; paid tier skips metering entirely |
| License file deleted/corrupted at runtime | Unexpected tier downgrade | Graceful degradation to free tier with clear UI warning; periodic re-check (not just on startup) |
| Clock skew causes premature expiry | False expiry on user's machine | Add 24-hour grace period to expiry check; document NTP requirement |
| Free-tier users hitting limits abruptly | Poor UX, churn | Warning banners at 80% and 95% quota; clear CTA to upload license |
| Existing installations have no license | Migration UX | First boot without license = free tier; no breaking change for existing users |
