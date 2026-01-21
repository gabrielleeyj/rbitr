# EPIC_2.md — React Control Plane + Policy Lifecycle

## Executive intent

Epic 2 productizes the Gateway (Epic 1) into an **operable control plane**:

- A **React Admin Console** for day-2 operations (policy, tools, risk overrides, evidence)
- **Policy lifecycle** (versioning, publish/rollback, simulation) to make governance changes safe
- **Admin audit trail** stored in DB for “who changed what, when” (CISO-ready)

---

## Decisions (locked for Epic 2)

### Auth header for UI → Admin API

**Use:** `Authorization: Bearer <admin_key>`

Notes:

- Admin keys already exist in DB with scopes and are looked up by key hash (admin_key_id + scopes). :contentReference[oaicite:0]{index=0}
- Server MAY support `X-Admin-Key` temporarily for backward compat, but UI should standardize on Bearer.

### Admin audit storage

**Use DB table:** `rbitr.admin_audit_events` (defined below)

Rationale:

- Logs-only is insufficient for governance/compliance UX; we need queryable events per tenant and exportable evidence.

---

## Scope

### In scope

1. **React Admin Console**

- Tenant selection
- Evidence viewer (list + detail + filters)
- Policy versions (view/create/publish/rollback)
- Policy editor (basic text editor)
- Risk overrides (list/upsert/delete)
- Tools config (list/update)
- Settings: admin write lock toggle
- Audit viewer (recent admin events)

2. **Policy lifecycle**

- Store policy versions as immutable records
- Set active policy pointer per tenant
- Publish and rollback flows
- Policy simulation endpoint (evaluate without executing tool calls)

3. **Admin audit trail**

- Write audit events for all mutating admin actions

### Out of scope (Epic 3+)

- Approval execution workflow (approve → replay tool call)
- Broad SDKs / MCP adapter / connector marketplace
- Hosted SaaS control plane

---

## API plan (Epic 2)

### Existing (Epic 1) relevant endpoints

Public:

- `POST /v1/tools/{tool_id}/call`
- `GET /v1/tenants/{tenant_id}/evidence?limit=50` :contentReference[oaicite:1]{index=1}

Admin (already present in some form):

- `PUT /admin/tenants/{tenant_id}/config`
- `PUT /admin/tenants/{tenant_id}/tools/{tool_id}`
- `PUT /admin/tenants/{tenant_id}/policy`
- `PUT /admin/tenants/{tenant_id}/risk-overrides/{action_type}`
- `PUT /admin/bootstrap/complete`

### Add / evolve endpoints for a real control plane

**Tenants**

- `GET /admin/tenants` (list)
- `GET /admin/tenants/{tenant_id}` (details)

**Evidence (admin-safe)**

- Preferred: `GET /admin/tenants/{tenant_id}/evidence?limit=&decision=&action_type=&risk=&since=`
- Option: reuse public evidence endpoint but require admin auth for richer filters.

**Policy lifecycle**

- `GET /admin/tenants/{tenant_id}/policies` (list versions, includes active flag)
- `GET /admin/tenants/{tenant_id}/policies/{policy_version}` (get module + metadata)
- `POST /admin/tenants/{tenant_id}/policies` (create new version)
- `PUT /admin/tenants/{tenant_id}/policies/{policy_version}/publish` (set active)
- `PUT /admin/tenants/{tenant_id}/policies/rollback` (rollback to previous or specified)

**Policy simulation**

- `POST /admin/tenants/{tenant_id}/policies/simulate`
  - Input: policyInput-like object (no tool execution)
  - Output: Decision object + validation result

**Risk overrides**

- `GET /admin/tenants/{tenant_id}/risk-overrides`
- `PUT /admin/tenants/{tenant_id}/risk-overrides/{action_type}` (upsert)
- `DELETE /admin/tenants/{tenant_id}/risk-overrides/{action_type}`

**Tools**

- `GET /admin/tenants/{tenant_id}/tools`
- `PUT /admin/tenants/{tenant_id}/tools/{tool_id}`

**Settings**

- `GET /admin/settings`
- `PUT /admin/settings/admin-write-lock` (toggle)

**Audit**

- `GET /admin/tenants/{tenant_id}/audit?limit=50`
- Optional global: `GET /admin/audit?limit=50`

---

## Data model

### A) Policy versions (recommended)

- `rbitr.policy_versions` (immutable per version)
- `rbitr.tenants.active_policy_version` (pointer)

(If you already store a single policy row per tenant today, migrate to versioned rows.)

### B) Admin audit events (required in Epic 2)

See schema below.

### C) Admin write lock (recommended)

- Already planned as system setting; enforce across mutating admin endpoints.

---

## Exact DB schema: `rbitr.admin_audit_events`

### Migration file

`migrations/0000X_add_admin_audit_events.sql`

### SQL

CREATE TABLE IF NOT EXISTS rbitr.admin_audit_events (
audit_event_id TEXT PRIMARY KEY,
tenant_id TEXT NULL,

-- Actor (admin key-based today; keep flexible for future JWT/users)
actor_type TEXT NOT NULL DEFAULT 'admin_key',
actor_id TEXT NULL, -- e.g., admin_key_id
actor_display TEXT NULL, -- optional display string

-- What happened
action TEXT NOT NULL,
resource_type TEXT NOT NULL,
resource_id TEXT NULL,

-- Change payloads (MUST be redacted; never store secrets)
before JSONB NULL,
after JSONB NULL,

-- Request context (for correlation)
request_id TEXT NULL,
ip INET NULL,
user_agent TEXT NULL,

created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

-- Basic format constraints (keep permissive, but consistent)
CONSTRAINT action*format_chk CHECK (action ~ '^[A-Z0-9]+(\\.[A-Z0-9*]+)+$'),
  CONSTRAINT resource_type_chk CHECK (resource_type ~ '^[A-Z0-9]+(\\.[A-Z0-9_]+)*$')
);

-- Query patterns: per-tenant recent events + global recent events
CREATE INDEX IF NOT EXISTS idx_admin_audit_events_tenant_time
ON rbitr.admin_audit_events (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_time
ON rbitr.admin_audit_events (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_action
ON rbitr.admin_audit_events (action);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_resource
ON rbitr.admin_audit_events (resource_type, resource_id);

### Implementation notes

- Generate `audit_event_id` in Go (e.g., `ae_<uuid>`), consistent with your existing id patterns.
- **Never** store tool auth secrets in `before/after`. If a tool config changes, store:
  - base_url, auth_type, enabled flags
  - auth_value should be omitted or replaced with `"***redacted***"`.

---

## Event naming convention (must stay consistent as features grow)

### Format

Use a stable, dot-delimited namespace:

**`<DOMAIN>.<ENTITY>.<VERB>`** (optionally add one sub-entity)

- DOMAIN: `TENANT`, `TOOL`, `POLICY`, `RISK_OVERRIDE`, `SETTINGS`, `BOOTSTRAP`, `ADMIN`
- ENTITY: `CONFIG`, `VERSION`, `ACTIVE`, etc.
- VERB: `CREATE`, `UPDATE`, `UPSERT`, `DELETE`, `PUBLISH`, `ROLLBACK`, `SET`, `COMPLETE`, `ROTATE`

Rules:

- Uppercase, underscore allowed within tokens
- Verb at end
- Prefer “what changed” over “how it changed”
- Avoid embedding IDs in action names (IDs belong in resource_id)

### Resource type convention

Use `resource_type` aligned to domain/entity:

- `TENANT`
- `TENANT.CONFIG`
- `TOOL`
- `POLICY.VERSION`
- `POLICY.ACTIVE`
- `RISK_OVERRIDE`
- `SETTINGS`
- `BOOTSTRAP`

### Canonical event list (Epic 2)

| Action                          | resource_type    | resource_id               | before/after expectation                                            |
| ------------------------------- | ---------------- | ------------------------- | ------------------------------------------------------------------- |
| `TENANT.CONFIG.UPDATE`          | `TENANT.CONFIG`  | `{tenant_id}`             | before/after of name, key_hash updated (no raw key)                 |
| `TOOL.CONFIG.UPDATE`            | `TOOL`           | `{tool_id}`               | before/after base_url/auth_type/enabled (no auth_value)             |
| `POLICY.VERSION.CREATE`         | `POLICY.VERSION` | `{policy_version}`        | after includes metadata (created_by, notes); rego omitted or hashed |
| `POLICY.VERSION.PUBLISH`        | `POLICY.ACTIVE`  | `{policy_version}`        | before/after active_policy_version                                  |
| `POLICY.VERSION.ROLLBACK`       | `POLICY.ACTIVE`  | `{target_policy_version}` | before/after active_policy_version                                  |
| `RISK_OVERRIDE.UPSERT`          | `RISK_OVERRIDE`  | `{action_type}`           | before/after risk enum                                              |
| `RISK_OVERRIDE.DELETE`          | `RISK_OVERRIDE`  | `{action_type}`           | before contains removed override                                    |
| `SETTINGS.ADMIN_WRITE_LOCK.SET` | `SETTINGS`       | `admin_write_lock`        | before/after true/false                                             |
| `BOOTSTRAP.COMPLETE`            | `BOOTSTRAP`      | `bootstrap_complete`      | before/after true/false                                             |

### Future-proof extensions (examples)

- `ADMIN.KEY.ROTATE`
- `TENANT.CREATE`
- `TENANT.DELETE`
- `APPROVAL.REQUEST.RESOLVE` (Epic 3)
- `POLICY.SIMULATE.RUN` (optional; may be noisy—log-only unless needed)

---

## UI requirements (React)

### Pages

1. **Tenants**

- list/select tenant
- show active policy version

2. **Evidence**

- list + filters (decision/action_type/risk/time)
- detail drawer showing decision/rule/reasons + hashes
- download export

3. **Policies**

- versions list (active highlighted)
- view rego
- create version (notes)
- publish/rollback
- simulate (optional panel)

4. **Risk Overrides**

- list/upsert/delete

5. **Tools**

- list/update base_url/auth_type/enabled

6. **Settings**

- admin write lock toggle
- audit viewer

7. **Audit**

- tenant-scoped audit events table (time, action, actor, resource, diff summary)

### UI auth

- Admin key input stored in localStorage for dev
- UI sends `Authorization: Bearer <admin_key>` on every admin API call

---

## Backend requirements (Epic 2)

### Admin auth middleware

- Parse `Authorization: Bearer <token>`
- Hash token and lookup admin key record (scopes) :contentReference[oaicite:2]{index=2}
- Enforce `admin:read` vs `admin:write` by endpoint method

### Admin write lock enforcement

- For all mutating endpoints, check `admin_write_lock == true` → reject with 423 (Locked) or 403

### Audit emission

For every mutating admin endpoint:

- Compute a safe `before` snapshot (redacted)
- Apply update
- Compute safe `after` snapshot
- Insert audit event in the same request path
- Include `request_id` (use existing X-Request-Id correlation), plus IP/user agent

---

## Testing (Epic 2)

- Handler tests:
  - Bearer auth parsing + scope enforcement
  - admin write lock blocks writes
  - audit events inserted on each mutating endpoint
- Integration:
  - create policy version → publish → tool call behavior changes → evidence updated
  - risk override upsert reflected in subsequent classification/evaluation
- UI smoke:
  - load tenants, view evidence, view policies, publish policy, view audit event

---

## Non-functional requirements

- No secrets in audit table (ever)
- No high-cardinality labels in Prometheus for audit/policy errors (log tenant/policy_version instead)
- Response bodies for evidence remain export-whitelisted (carry over Epic 1 stance)

---
