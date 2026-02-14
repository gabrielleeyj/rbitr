# EPIC 7 — Multi-Tenant Gateway & Agent Identity Architecture

## Goal

Harden rbitr into a secure multi-tenant, multi-agent governance platform/gateway.

Ensure:

- Multiple tenants can safely share the same gateway deployment.
- Each tenant has isolated tools, policies, approvals, audit, and evidence.
- Multiple agents per tenant can send traffic concurrently.
- Tenant routing is clean, secure, and scalable.
- Configuration lookups are efficient (caching).
- No cross-tenant data leakage is possible.
- Safe onboarding and rotation
- Horizontal scalability
- SOC2 / security review
- Revenue deployment

Non-Goals

- No per-tenant dedicated deployments (single shared gateway model).
- No full RBAC within tenant (future epic).
- No per-agent policy (future epic).
- No SSO/SCIM integration yet.
- No billing enforcement.

---

## Architectural Decisions

1. Tenant resolved via **Bearer Tenant Key**

- `Authorization: Bearer <tenant_key>`

2. Agent identified via optional header:

- `X-Agent-ID`

3. Public API standardization:

- `POST /v1/tools/{tool_id}/call`

4. Tenant config cached in memory with TTL.
5. All governance decisions always scoped by `tenant_id`.
6. Stateless gateway instances.

---

## Story 1 - Tenant Identity & Authentication Standardization

### Description

Standardize how the gateway identifies and authenticates tenants.

### Tasks

- Enforce `Authorization: Bearer <tenant_key>` on all public endpoints.
- Remove any fallback implicit tenant resolution.
- Add tenant lookup by:
  - `tenant_key -> tenant_id`
- Validate:
  - tenants exists
  - tenant not disabled (future-proof flag)
- Ensure constant-time hash comparison for keys.
- Reject revoked/rotated keys.

### Acceptance Criteria

- All public calls require valid Bearer token.
- Invalid tenant key returns `401`.
- No request proceeds without resolved `tenant_id`.
- Structured logs include `tenant_id`.
- No endpoint bypasses auth

---

## Story 2 — Tenant API Key Lifecycle Management (NEW)

### Description

Provide secure creation, rotation, and revocation of tenant API keys.

### Tasks

DB

- Add rbitr.tenant_keys table:

```postgresql
CREATE TABLE rbitr.tenant_keys (
id UUID PRIMARY KEY,
tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),

key_hash BYTEA NOT NULL,
key_prefix TEXT NOT NULL,

created_at TIMESTAMP NOT NULL DEFAULT now(),
rotated_at TIMESTAMP NULL,
revoked_at TIMESTAMP NULL,

UNIQUE(key_hash)
);
```

Key Generation

- Generate 32+ bytes of entropy
- Prefix: `rbtr_live_`
- Hash using HMAC-SHA256(server_secret)

Admin APIs

- POST /admin/tenants → create tenant + key
- POST /admin/tenants/{id}/keys/rotate
- POST /admin/tenants/{id}/keys/{key_id}/revoke
- GET /admin/tenants/{id}/keys (no secrets)

Auth Middleware

- Extract Bearer token
- Hash
- Lookup in tenant_keys
- Verify not revoked

Audit

- Emit events:
  - `TENANT.KEY.CREATED`
  - `TENANT.KEY.ROTATED`
  - `TENANT.KEY.REVOKED`

### Acceptance Criteria

- Raw keys shown only once
- Revoked keys rejected immediately
- Rotation invalidates old keys
- All actions audited

---

## Story 3 — Agent Identity Support

### Description

Support multiple agents within a tenant for audit, traceability, and analytics.

### Tasks

- Accept optional header:
  - `X-Agent-ID`
- Validate length + allowed charset.
- Store `agent_id` in:
  - ADR records
  - approval_requests
  - evidence export DTO
- Add to structured logs.
- Add metric dimension (low cardinality safe use).

### Acceptance Criteria

- Agent ID recorded in audit trail.
- Missing Agent ID does not block execution.
- Agent ID never used as policy dimension yet (future).

---

## Story 4 — Multi-Tenant Isolation Guarantees

### Description

Ensure hard isolation between tenants.

### Tasks

- Audit all DB queries:
  - Must include `tenant_id` filter.
- Add integration test:
  - Tenant A cannot access Tenant B evidence.
- Validate approvals:
  - Approval tokens scoped to tenant.
- Validate tool lookup:
  - Tool must belong to tenant.

### Acceptance Criteria

- Cross-tenant access always rejected.
- Integration tests simulate two tenants.
- Approval replay across tenants impossible.

---

## Story 5 — Config & Policy Caching Layer

### Description

Reduce DB load while maintaining correctness.

### Design

In-memory cache with TTL (e.g. 30–60 seconds) for:

- Tenant config
- Active policy version
- Tool registry
- Risk overrides

### Tasks

- Add cache struct:
  - keyed by `tenant_id`
- Store:
  - active policy
  - tool configs
  - risk overrides
- Add TTL invalidation.
- Add admin-triggered cache bust endpoint (optional).
- Metrics:
  - `config_cache_hits_total`
  - `config_cache_miss_total`

### Acceptance Criteria

- Gateway does not hit DB on every request.
- Cache respects TTL.
- No stale config beyond TTL.
- Admin policy publish invalidates cache (optional stretch).

---

## Story 6 — Multi-Agent Concurrency Safety

### Description

Ensure gateway handles many agents safely.

### Tasks

- Validate approval state transitions under concurrency.
- Add integration test:
  - Two agents attempt same approval execution.
- Confirm:
  - single execution guarantee.
- Add DB constraint (if not already):
  - approval executed_at must be NULL before update.

### Acceptance Criteria

- Exact-once execution preserved.
- Concurrent resubmits safe.
- No race conditions produce duplicate tool calls.

---

## Story 7 — Gateway Scaling Model

### Description

Prepare gateway for horizontal scaling.

### Tasks

- Confirm no in-memory state is required for correctness.
- Ensure:
  - approvals
  - decisions
  - tokens
    all persisted in DB.
- Add advisory lock for background tasks (if not already).
- Add readiness endpoint for container orchestration.

### Acceptance Criteria

- Multiple gateway instances behind LB work correctly.
- No instance affinity required.
- Stateless design validated.

---

## Story 8 — Tenant Management Improvements (Optional but Strategic)

### Description

Make tenant creation and management first-class.

### Tasks

- Add admin endpoint:
  - `POST /admin/tenants`
- Generate:
  - `tenant_id`
  - `tenant_key`
- Add enable/disable flag.
- Add rotation endpoint for tenant key.

### Acceptance Criteria

- New tenant can be created via API.
- Tenant key rotation invalidates old key.
- Disabled tenant cannot execute tool calls.

---

## Story 9 — Observability by Tenant & Agent

### Description

Improve visibility for SaaS-grade operations.

### Tasks

- Add metrics:
  - `gateway_requests_total{tenant}`
  - `gateway_requests_total{tenant,decision}`
  - `approvals_created_total{tenant}`
- Ensure low cardinality (do NOT label by agent_id).
- Add structured log fields:
  - `tenant_id`
  - `agent_id`
  - `tool_id`
  - `decision`

### Acceptance Criteria

- Per-tenant metrics available.
- No high-cardinality metric explosion.

---

## Data Model Adjustments

### tenants

```postgresql
ALTER TABLE rbitr.tenants
ADD COLUMN enabled BOOLEAN DEFAULT true
ADD COLUMN rotated_at TIMESTAMP NULL
ADD COLUMN tenant_plan TEXT NULL;
```

### tenant_keys

(see story 2)

Add to ADR / approvals tables:

```postgresql
ALTER TABLE rbitr.approval_requests
ADD COLUMN agent_id TEXT NULL;
```

---

## Final Acceptance Criteria (Epic-Level)

- Multiple tenants can share one gateway.
- Each tenant:
  - has isolated tools
  - isolated policy
  - isolated approvals
  - isolated evidence
- Multiple agents per tenant work concurrently.
- Gateway horizontally scalable.
- Config caching reduces DB pressure.
- No cross-tenant leakage possible.

---

## What This Enables Strategically

This epic transforms rbitr from:

“Cool governance proxy”

into:

“Multi-tenant SaaS-ready AI control plane”

Without this, you cannot:

- Onboard real customers
- Sell via AWS/GCP Marketplace
- Support production multi-org traffic

---

## What is a Tenant Key in rbitr?

A **tenant key** is:

The API credential that identifies and authenticates a customer’s agents to your gateway.

It’s equivalent to:

- Stripe API key
- OpenAI API key
- AWS access key (simplified)

Example:

```
rbtr_live_k2x93nd9A8sHfLQp7RzE...
```

Agents send it as:

```
Authorization: Bearer rbtr_live_...
```

You map:

```
tenant_key → tenant_id → config/policy/tools
```

---

## You should NEVER store raw tenant keys

Like passwords, API keys must be stored hashed.

### Pattern

When generating:

```
raw*key = random(32 bytes)
display_key = "rbtr_live*" + base64url(raw_key)
hash = HMAC_SHA256(server_secret, display_key)
```

DB stores:

```
tenant_key_hash
```

Never the raw key.

You only show the raw key once on creation/rotation.

---

## Minimum key lifecycle you need

For production:
| Action | Required? | Why |
| ------------ | --------- | --------------- |
| Generate | ✅ | Onboarding |
| Hash & store | ✅ | Security |
| Verify | ✅ | Auth |
| Rotate | ✅ | Breach response |
| Revoke | ✅ | Offboarding |
| Expire | ⭕ (later) | Enterprise |

---

## Where keys should live (DB schema)

Add a table (or extend existing tenant table):

```postgresql
CREATE TABLE rbitr.tenant_keys (
  id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),

  key_hash BYTEA NOT NULL,
  key_prefix TEXT NOT NULL, -- rbtr_live_

  created_at TIMESTAMP NOT NULL DEFAULT now(),
  rotated_at TIMESTAMP NULL,
  revoked_at TIMESTAMP NULL,

  UNIQUE(key_hash)
);

```

Why prefix?
So in logs you can identify which key was used without leaking it:

```
rbtr_live_ab12****
```

---

## Admin API: what you should implement

### Create tenant + key

```
POST /admin/tenants
```

Returns (only once):

```json
{
  "tenant_id": "t_92ks8a",
  "api_key": "rbtr_live_k2x93nd9..."
}
```

### Rotate key

```
POST /admin/tenants/{tenant_id}/keys/rotate
```

Returns new key, revokes old.

### Revoke key

```
POST /admin/tenants/{tenant_id}/keys/{key_id}/revoke
```

Immediate invalidation.

### List keys (no secrets)

```
GET /admin/tenants/{tenant_id}/keys
```

Returns:

```json
{
  "id": "uuid",
  "prefix": "rbtr_live_",
  "created_at": "...",
  "revoked_at": null
}
```

---

## Gateway auth flow (runtime)

On every request (example):

```go
func authenticate(r *http.Request) (*Tenant, error) {
  raw := extractBearerToken(r)

  hash := HMAC(secret, raw)

  key := store.GetTenantKeyByHash(hash)

  if key == nil || key.RevokedAt != nil {
    return nil, ErrUnauthorized
  }

  return store.GetTenant(key.TenantID)
}
```

Cache this lookup.

---
