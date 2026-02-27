# EPIC 8 — Enforcement Hardening (Rate Limits, Argument Rules, Shadow Mode, Explainability)

## Summary

Epic 8 hardens **rbitr** from a governed gateway into a production-grade **AI tool enforcement layer** by adding:

1. **Deterministic enforcement controls** teams expect from an "AI firewall":
   - multi-window **rate limits & budgets** (burst + daily)
   - first-class **argument-level constraints**
   - deterministic **rule priority / conflict resolution**
   - **explainability** (“why was this blocked?”) surfaced in ADR + UI

2. **Safe adoption mechanics**:
   - **shadow mode** (evaluate + log, but don’t block)
   - upgrade hardening: admin scopes, key hashing upgrades, cache invalidation, MCP pass-through routing

This epic is intentionally shaped to close the main parity gaps with proxy-style competitors while leaning into rbitr’s control-plane strengths (approvals, audit immutability, lifecycle).

---

## Implementation Status (2026-02-27)

Implemented in codebase:

- Story 1: Multi-window rate limits and budgets (tenant/agent/tool/action scope), including REST `429` and MCP `-32005` enforcement paths with metrics.
- Story 2: First-class argument constraints (`allow`/`deny`, ops including `eq|prefix|regex|in|contains|jsonschema`), with deny reasons and ADR constraint-failure payloads.
- Story 3: Deterministic rule resolution and explainability (`matched_rules` parsing/sorting, tie-break semantics, reason propagation).
- Story 4: Shadow mode (`enforce|shadow`) with execute-but-log deny behavior for REST and MCP.
- Story 5: Granular admin scope enforcement across control-plane routes, plus UI scope-aware visibility/disable behavior.
- Story 6: Tenant key hashing upgrade to HMAC-SHA256 with multi-secret verification and legacy SHA-256 lazy-upgrade path.
- Story 7: Cache invalidation hardening via tenant config versioning and versioned cache keys for tool/risk lookups.
- Story 8: Explicit MCP pass-through upstream selection (`mcp_passthrough_upstream_tool_id`) with deterministic fallback + metric/log when unset.

Remaining from this epic:

- Stretch Story 9 (MCP SSE streaming mode) is not yet implemented.
- Documentation cleanup is still needed to align all operator docs with the delivered behavior and defaults.

---

## Goals

- Add **multi-window rate limiting** per tenant/agent/tool/action.
- Make **argument-level constraints** a product surface (DecisionV2 constraints schema + enforcement + UI + ADR/audit).
- Ensure deterministic evaluation under conflict via **rule priority** and clear tie-break rules.
- Introduce **shadow mode** to allow safe rollout with measurable impact.
- Improve operational readiness:
  - enforce **admin scopes** beyond admin:read/write
  - upgrade **tenant key hashing** to HMAC-SHA256
  - add **cache invalidation** on admin writes (reduces stale policy/tool/risk)
  - make MCP **pass-through routing** explicit for multi-upstream tenants

## Non-Goals (explicitly out of scope)

- Full SSE streaming support for MCP Streamable HTTP (may be a stretch story).
- Full OPA trace capture (too heavy/noisy); we will store a concise explanation payload.
- Redis dependency required in v1 (we start Postgres-backed limits; Redis optional later).
- Capability-aware routing for every MCP method (we’ll implement explicit upstream selection first).

---

## Current Baseline (as of Epics 1–7)

- OPA evaluator already:
  - loads policy from DB, includes `policy_version` in input
  - supports rule priority in decision output (`models.DecisionRule{ID, Priority}`)
  - has a prepared-query cache with TTL
- Audit trail:
  - admin audit events are hash-chained + immutable via trigger
  - retention cleanup job exists
- Approvals:
  - token gated, claim-before-execute semantics (`APPROVED -> EXECUTING -> EXECUTED|FAILED`)
- MCP:
  - `tools/list` + `tools/call` governed execution
  - pass-through currently routes to first MCP tool by deterministic ordering

Epic 8 builds on these foundations.

---

## Proposed Stories

### Story 1 — Multi-window Rate Limits & Budgets (tenant/agent/tool)

**Problem**: Agents can loop, burst, or incur runaway cost. Teams expect burst + daily guardrails.

**Deliverables**

- A rate limiter that supports **multiple windows** (at minimum: per-minute + per-day), with counters keyed by:
  - tenant_id (required)
  - agent_id (optional)
  - tool_id (optional)
  - action_type (optional)
- Enforcement happens **outside agent context** and is deterministic.

**Policy Integration (DecisionV2 constraints)**

- Support policy returning constraints:
  - `constraints.rate_limit.per_minute`
  - `constraints.rate_limit.per_day`
  - `constraints.rate_limit.scope` (e.g. `tenant|tenant_agent|tenant_tool|tenant_agent_tool`)
- Precedence:
  1. policy constraints
  2. tenant config defaults
  3. system defaults

**Implementation Notes**

- Start with Postgres-backed counters (transactional upsert).
- If needed later: add Redis implementation behind an interface.

**DB Changes (suggested)**

- `rbitr.rate_limit_counters` (rolling window counters)
  - `tenant_id TEXT NOT NULL`
  - `agent_id TEXT NULL`
  - `tool_id TEXT NULL`
  - `action_type TEXT NULL`
  - `window TEXT NOT NULL` (e.g. `minute`, `day`)
  - `bucket_start TIMESTAMPTZ NOT NULL`
  - `count BIGINT NOT NULL`
  - `updated_at TIMESTAMPTZ NOT NULL`
  - PK on `(tenant_id, agent_id, tool_id, action_type, window, bucket_start)`

**API/Behavior**

- On public REST and MCP tool calls:
  - evaluate policy
  - derive rate-limit configuration
  - **check+increment** counters
  - if exceeded → return deny-like response:
    - REST: 429 with safe error body
    - MCP: JSON-RPC error `-32005` (new) with `limit`, `window`, and `retry_after_seconds`

**Metrics**

- `rate_limit_checks_total{result,window}`
- `rate_limit_exceeded_total{window,scope}`
- `rate_limit_latency_ms`

**Acceptance Criteria**

- Burst and daily limits work together (minute limit triggers while day limit still under cap).
- Limits are enforced for both REST and MCP paths.
- Counters reset correctly at bucket boundaries.
- Tests cover:
  - per-minute bucket rollover
  - per-day bucket rollover
  - scope variants
  - concurrency correctness (no double increments beyond limit)

---

### Story 2 — First-class Argument-level Constraints

**Problem**: Tool-level allow/deny is too blunt. Real-world governance needs argument-level rules.

**Deliverables**

- Standardize a **constraints schema** in DecisionV2 for argument rules.
- Enforce those constraints deterministically in gateway **after** policy evaluation but **before** tool execution.

**Constraints Schema (proposed, minimal v1)**

- `constraints.args.allow` — list of rules
- `constraints.args.deny` — list of rules

Each rule:

- `path`: JSON pointer-ish path (e.g. `"/branch"`, `"/repo/name"`)
- `op`: one of `eq | prefix | regex | in | contains | jsonschema`
- `value`: scalar or list or schema
- `message`: optional safe reason

**Enforcement Semantics**

- Evaluate deny rules first; then allow rules.
- If allow rules exist for a path/op, the input must match at least one allow rule.
- On violation → treat as denied with a clear reason code:
  - `ARG_CONSTRAINT_DENY` / `ARG_CONSTRAINT_NOT_ALLOWED`

**ADR/Audit Explainability**

- Persist a compact explanation:
  - `constraint_failures[]`: `{path, op, reason_code, rule_id?}`

**UI**

- In approvals detail + evidence export views:
  - show evaluated constraints
  - show which argument rule failed
  - display a compact arguments summary (redacted)

**Acceptance Criteria**

- Constraints apply equally to REST and MCP.
- Internal control args (e.g. `_rbitr_approval_token`) are excluded before evaluation/hashing.
- Tests cover regex/prefix/eq and nested argument paths.

---

### Story 3 — Deterministic Rule Priority & Conflict Resolution + “Matched Rules” Explain

**Problem**: As policies grow, operators need deterministic outcomes and confidence.

**Deliverables**

- Formalize priority/conflict semantics:
  - Higher `priority` wins (integer)
  - Tie-break: explicit `DENY > REQUIRE_APPROVAL > ALLOW` (or other fixed ordering)
- Persist “what happened” in ADR:
  - `matched_rules[]`: `{rule_id, priority, effect, reasons[], constraints_summary}`
  - keep payload small; do not store full OPA trace

**Policy Authoring UX (Admin Console)**

- In simulate + publish checklist:
  - show top matched rule
  - show effective decision and why

**Acceptance Criteria**

- Given multiple matching rules, system chooses deterministically.
- ADR contains consistent matched-rules payload.
- Export redaction allowlist remains safe (no secrets/tokens).

---

### Story 4 — Shadow Mode (Evaluate + Log, Don’t Block)

**Problem**: Teams need safe rollout. Over-restrictive policies cause rapid abandonment.

**Deliverables**

- Tenant-level enforcement mode:
  - `enforce` (default)
  - `shadow`
- In shadow:
  - still evaluate policy
  - still compute decision, constraints, reasons
  - still write ADR/audit/metrics
  - but tool execution proceeds (with optional exception for “hard deny” categories if configured)

**DB/Config Changes (suggested)**

- Add to `tenant_config`:
  - `enforcement_mode TEXT NOT NULL DEFAULT 'enforce' CHECK IN ('enforce','shadow')`

**UI**

- Tenant settings toggle with warning
- Simple reporting:
  - “Shadow would have blocked N calls in last 24h” (can be computed from ADR decisions)

**Acceptance Criteria**

- Shadow mode produces the same ADR payload as enforce mode.
- Enforcement behavior changes only at the final block/allow stage.
- Tests validate:
  - deny decision still forwards in shadow
  - approvals still behave predictably (recommended: approvals still enforced even in shadow, unless explicitly configured)

---

### Story 5 — Admin Scope Enforcement (beyond admin:read/admin:write)

**Problem**: Admin keys currently store scopes but enforcement is coarse.

**Deliverables**

- Define granular scopes (suggested):
  - `admin:tenants:read|write`
  - `admin:policies:read|write|publish|rollback|simulate`
  - `admin:tools:read|write`
  - `admin:approvals:read|decide`
  - `admin:audit:read|export`
  - `admin:notifications:read|write|test`
  - `admin:settings:read|write`
- Add middleware/helper to require scope per route.
- UI hides/disables actions when scope missing.

**Acceptance Criteria**

- Unauthorized scope returns 403 with safe body.
- Tests cover at least one endpoint per scope family.

---

### Story 6 — Tenant Key Hashing Upgrade (HMAC-SHA256)

**Problem**: Plain SHA-256 hashing is OK internally but weaker for production if DB leaks.

**Deliverables**

- Hash tenant keys using HMAC-SHA256 with server secret:
  - `key_hash = HMAC(secret, raw_key)`
- Support secret rotation:
  - allow `RBTR_TENANT_KEY_HMAC_SECRETS=secretA,secretB` (verify against all; generate with first)
- Migration path:
  - keep existing hashes temporarily; on successful auth with legacy hash, re-hash+store new format (or do a one-time rehash job if raw keys are available, likely not)

**Acceptance Criteria**

- New keys are stored using HMAC hash.
- Auth verifies using configured secrets.
- Tests cover multi-secret verification.

---

### Story 7 — Cache Invalidation on Admin Writes

**Problem**: TTL-only caches can cause confusing “stale policy/tool/risk” windows.

**Deliverables**

- Add a `tenant_config_version` integer that increments on admin writes affecting:
  - tools
  - policy publish/rollback
  - risk overrides
- Cache keys include version; or caches are explicitly invalidated when version changes.
- Ensure both REST and MCP use updated cached lookups.

**DB Changes (suggested)**

- `tenant_config.version BIGINT NOT NULL DEFAULT 1`

**Acceptance Criteria**

- After publishing a policy, next request sees new policy immediately (no waiting for TTL).
- Tests cover invalidation across policy and tool updates.

---

### Story 8 — MCP Pass-through Routing: Explicit Upstream Selection

**Problem**: Pass-through methods currently route to the first MCP tool by `tool_id`, which breaks multi-upstream tenants.

**Deliverables**

- Add tenant config `mcp_passthrough_upstream_tool_id`.
- If set: route pass-through methods to that tool’s `mcp_upstream_url`.
- If unset:
  - keep current deterministic behavior but emit warning + metric.

**Acceptance Criteria**

- Multi-upstream tenants can explicitly control pass-through routing.
- Tests cover explicit selection and unset fallback.

---

### Stretch Story 9 — MCP Streamable HTTP SSE Support (Long-running calls)

**Deliverables**

- Implement `GET /v1/mcp/{tenant_id}` stream mode for SSE.
- Add safe limits:
  - max stream duration
  - max bytes
  - heartbeat

**Acceptance Criteria**

- SSE works for long-running upstream calls.
- Deny/approval-required responses still return immediately with appropriate JSON-RPC error payload.

---

## API & Error Contract Additions

### REST

- 429 for rate limit exceeded:
  - `{ code: "RATE_LIMIT_EXCEEDED", window: "minute|day", retry_after_seconds: n }`

### MCP

- New JSON-RPC error code:
  - `-32005` Rate limit exceeded
  - data: `{ window, limit, remaining, retry_after_seconds, scope }`

---

## Rollout Plan

1. Ship Story 4 (Shadow Mode) early so teams can turn it on before tightening.
2. Ship Story 1 (Rate Limits) next with conservative defaults.
3. Ship Story 2+3 (Arg constraints + priority + explainability) together to avoid “mystery blocks”.
4. Harden admin surface (Story 5) and key hashing (Story 6).
5. Improve operational correctness (Story 7+8).

Recommended release toggles:

- `RBTR_FEATURE_RATE_LIMITING=true`
- `RBTR_FEATURE_ARG_CONSTRAINTS=true`
- `RBTR_FEATURE_SHADOW_MODE=true`

---

## Testing Strategy

- Unit tests:
  - rate limiter bucket logic and atomic upserts
  - constraints evaluation on nested args and regex
  - rule priority tie-break semantics
  - scope middleware
  - HMAC hashing multi-secret verification
- Integration tests:
  - REST tool call rate limit exceeded (429)
  - MCP tool call rate limit exceeded (-32005)
  - shadow mode still forwards while recording deny decision in ADR
  - policy publish invalidates caches immediately
  - pass-through routing uses configured upstream

---

## Definition of Done

- All non-stretch stories (1–8) implemented with tests.
- ADR/evidence export and UI surfaces show:
  - decision + rule + reasons + constraints summary
  - rate-limit and arg-constraint failure explanations
- Metrics added and documented.
- README / CLAUDE.md updated with:
  - rate limiting semantics
  - constraints schema
  - enforcement mode behavior
  - admin scopes list
  - key hashing rotation strategy
