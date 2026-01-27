## EPIC 5 - SOC‑ready goals

- Completeness: every admin mutation, approval decision, and policy lifecycle event is logged.
- Integrity: logs are tamper‑evident and immutable.
- Attribution: actor, scope, and request context are always present.
- Retention: configurable retention + export path.
- Exportability: CSV/JSON export with stable schema and filters.

———

## 1) Make audit events immutable + append‑only

You already store in rbitr.admin_audit_events.

Add:

- immutable by policy (don’t update rows after insert).
- Ensure no updates are used in code (only INSERT).
- Consider DB trigger or permissions to block UPDATE/DELETE for the app role.

Action

- In DB: revoke UPDATE/DELETE on
  rbitr.admin_audit_events for the app user.
- Optional: add a DB trigger to reject updates.

———

## 2) Add tamper‑evidence (hash chain)

Add two columns:

- event_hash (hash of event payload + prev_hash)
- prev_hash (previous event hash for same tenant/global stream)

This creates a hash chain so tampering is detectable.

Implementation idea

- When inserting an event, read the last hash for that tenant (or global).
- event_hash = sha256(prev_hash + canonical_json(event))
- Store prev_hash + event_hash in row.

———

## 3) Ensure actor attribution is always present

Make sure:

- actor_type (admin_key, system, scheduler, etc.)
- actor_id (admin key id)
- actor_display (optional label)
- request_id, ip, user_agent

You already collect most of this. Fill any missing for scheduler/system events.

———

## 4) Standardize the audit event schema

Define a stable event schema and stick to it:

Required fields:

- audit_event_id, tenant_id, actor_type, actor_id, action, resource_type, resource_id,created_at

Recommended:

- before, after (redacted)
- request_id, ip, user_agent
- event_hash, prev_hash (if using chain)

This makes export clean and machine‑valid.

———

## 5) Redaction rules (SOC requirement)

SOC reviewers expect no secrets or raw payloads in audit logs.

Define a redaction allowlist:

- before/after should only include safe fields (non‑secret, no raw payloads).
- No secret refs resolved (only refs).
- No tokens, passwords, or raw headers.

This is mostly in place already; formalize it as a rule.

———

## 6) Export endpoint

Add an export endpoint that:

- returns CSV or JSON
- supports filters (tenant, action, resource_type, actor_id, date range)
- supports pagination and “download all”

Example:

- GET /admin/tenants/{tenant_id}/audit/export?format=csv&from=...&to=...

———

## 7) Retention policy

SOC requires clear retention.

Add a setting like:

- audit_retention_days (default 365 or 180)

Then:

- Add a scheduled cleanup job.
- Or leave events indefinitely (acceptable for early stage, but document it).

———

## 8) Documentation (SOC evidence)

In README / security doc:

- “Audit logs are append‑only”
- “Hash chaining enabled”
- “Retention = X days”
- “Export endpoints available”

———

# Minimal “SOC‑ready” implementation plan (small scope)

Fastest compliance path:

1. Add event_hash + prev_hash columns (migration).
2. Update insert logic in emitAuditEvent to compute hash chain.
3. Add export endpoint (JSON/CSV).
4. Document retention + no‑update policy.

Start implementation:

- DB migration for event_hash/prev_hash
- Store + handler changes
- Export endpoint + CSV support
- Tests for hash chain integrity

Tell me which parts you want first.
