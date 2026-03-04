# EPIC 1: Agent Governance Gateway + Action Semantics (Differentiated Control Plane)

## 0) Why this rework (differentiation vs Solo-style gateways)

Solo/Gloo/Envoy-class gateways are excellent at **HTTP routing, auth plumbing, rate limits, and generic policy enforcement**. To avoid building “just another gateway,” Epic 1 must introduce **agent governance semantics** early:

**Differentiators we add in Epic 1**

1. **Action Taxonomy & Classification**: turn raw API calls into _agent actions_ (e.g., `TICKET.CREATE`, `CRM.UPDATE_FIELD`, `DATA.EXPORT`, `PAYMENT.REFUND`, `ACCESS.GRANT`)
2. **Policy on Actions, not just endpoints**: rules expressed in terms of _action type + sensitivity + tenant context_
3. **Provable Governance Artifacts**: record a structured, exportable **Action Decision Record (ADR)** (not just access logs)
4. **Human Approval Hook (stub)**: REQUIRE_APPROVAL is not implemented end-to-end yet, but Epic 1 must produce a stable “approval required” record keyed to an **exact request hash**
5. **Tenant Evidence Preview**: a minimal “evidence export” for the last N actions (JSON) so the demo already feels like governance

Epic 1 still delivers a working gateway, but with early agent-governance “shape” so we’re not competing head-on with gateway vendors.

---

## 1) Epic 1 objective (end state)

By end of Epic 1, we can demo:

- An agent sends a tool call through our Gateway
- Gateway **classifies** the call into an **action type** (taxonomy)
- Gateway evaluates a policy based on **action type + risk tier** (allow/deny/approval-required)
- Gateway enforces ALLOW/DENY; for REQUIRE_APPROVAL it returns a placeholder response **and creates an Approval Request record** tied to the exact action hash
- Gateway writes an **Action Decision Record** for every attempt
- Admin can export last 50 decision records for a tenant (JSON “evidence preview”)

---

## 2) Reworked scope

### In scope (Epic 1)

**Gateway fundamentals**

- Go HTTP service + routing
- Tenant-scoped auth (API key), request IDs, idempotency key parsing
- Connector abstraction + generic REST connector
- PDP integration (simple internal engine in Epic 1; OPA later)

**Governance semantics (new)**

- Action taxonomy + classification engine
- Risk scoring from action type + method/path heuristics
- Action Decision Record (structured event) per request
- Approval-required stub: persists a pending Approval Request record keyed to request hash
- Evidence preview export: per-tenant recent decision records (JSON)

### Out of scope (still not Epic 1)

- Full approvals UI/inbox (Epic 3/5)
- Full append-only ledger + hash chaining (Epic 4; Epic 1 prepares fields)
- Multi-tenant hardening suite (Epic 3/4)
- Rich policy editor UI (Epic 5)
- Jira-specific connector beyond generic REST proxy (Epic 6)

---

## 3) Deliverables (Definition of Done)

1. Gateway service with endpoints:
   - `POST /v1/tools/{tool_id}/call`
   - `GET /v1/tenants/{tenant_id}/evidence?limit=50` (JSON evidence preview)
   - `GET /healthz`, `GET /metrics`
2. Action Classification:
   - Produces `action_type`, `action_risk`, `action_summary` for each request
3. Policy Engine (MVP-internal):
   - Action-based rules can return **ALLOW / DENY / REQUIRE_APPROVAL**
4. Approval-required stub:
   - On REQUIRE_APPROVAL, creates `approval_request` row and returns `409` with `approval_request_id`
5. Action Decision Records:
   - Stored for every attempted tool call (allowed/denied/approval-required)
6. Demo:
   - 3 scripted calls show:
     - Allowed: `TICKET.CREATE`
     - Approval required: `PAYMENT.REFUND`
     - Denied: `DATA.EXPORT`
   - Evidence preview export shows all three records with reasons

---

## 4) Core concepts (data model)

### 4.1 Action Taxonomy (v0)

Start with a small, opinionated set—expand later.

**Action Domains**

- `TICKET.*` (create/update/comment/close)
- `CRM.*` (read/update_field/update_record/delete)
- `DATA.*` (read/query/export/bulk_export)
- `PAYMENT.*` (refund/void/charge)
- `ACCESS.*` (grant/revoke/role_change)
- `ADMIN.*` (config_change/integration_change)

**Risk tier mapping (v0 default)**

- `ALLOW` default: low-risk reads, ticket comments
- `REQUIRE_APPROVAL`: refunds, role changes, bulk updates
- `DENY`: export customer data, permission grants, delete actions (until explicitly enabled)

### 4.2 Canonical Action Record (internal)

```json
{
  "request_id": "uuid",
  "tenant_id": "t_123",
  "agent_id": "a_456",
  "tool_id": "jira",
  "http_method": "POST",
  "path": "/rest/api/3/issue",
  "query": "",
  "body_hash": "sha256:...",
  "idempotency_key": "optional",
  "action_type": "TICKET.CREATE",
  "action_risk": "LOW|MEDIUM|HIGH|CRITICAL",
  "action_summary": "Create Jira issue in project ABC",
  "timestamp": "RFC3339"
}
```

### 4.3 Action Decision Record (ADR) — the governance artifact (new)

This is not generic access logging. It is a structured governance record.

```json
{
  "decision_id": "d_789",
  "request_id": "uuid",
  "tenant_id": "t_123",
  "agent_id": "a_456",
  "tool_id": "jira",
  "action_type": "TICKET.CREATE",
  "action_risk": "LOW",
  "decision": "ALLOW|DENY|REQUIRE_APPROVAL",
  "reason": "Policy: allow ticket creation",
  "rule_id": "rule_allow_ticket_create_v1",
  "policy_version": "p_v1",
  "request_hash": "sha256:...",
  "response_hash": "sha256:... (if executed)",
  "approval_request_id": "ar_123 (if required)",
  "timestamp": "RFC3339"
}
```

### 4.4 Approval Request (stub in Epic 1)

```json
{
  "approval_request_id": "ar_123",
  "tenant_id": "t_123",
  "agent_id": "a_456",
  "tool_id": "mock_internal",
  "action_type": "PAYMENT.REFUND",
  "request_hash": "sha256:...",
  "status": "PENDING|APPROVED|DENIED|EXPIRED",
  "expires_at": "RFC3339",
  "created_at": "RFC3339"
}
```

## 5) Stories and tasks breakdown (reworked)

### Story 1 (P0): Gateway request model + canonicalization

Goal: Normalize inbound tool calls into a canonical request structure with stable hashes and IDs.

### Tasks

- Router + endpoint: POST /v1/tools/{tool_id}/call
- Required headers: X-Tenant-Key, X-Agent-Id, optional X-Request-Id, Idempotency-Key
- Header allowlist filtering (drop cookies, auth headers, etc.)
- Body hashing + buffering with size limit (e.g., 256KB MVP)
- Compute request_hash as hash(canonical fields + body_hash) for approval binding
- Unit tests: canonicalization, hashing, header filtering

### Acceptance Criteria

- Canonical action can be generated for GET/POST calls
- request_hash is stable and deterministic across identical requests

### Story 2 (P0): Action taxonomy + classification engine (DIFFERENTIATOR)

Goal: Convert raw tool requests into meaningful agent actions.

### Tasks

1. Define ActionType enum + initial taxonomy table (v0)
2. Implement classifier interface:

- Inputs: tool_id, method, path, query, selected headers
- Output: action_type, action_risk, action_summary

3. Implement v0 classifiers:

- Generic classifier: method-based fallback
- Jira classifier (heuristic): map known Jira endpoints to TICKET.\*
- Mock internal tool classifier: map /refund, /export_customer_data, /change_role

4. Risk mapping:

- default risk by action_type
- allow override via config

5. Tests:

- Known requests map to expected action_type/risk/summary

### Acceptance Criteria

For demo paths, classification is correct and readable in logs/export

### Story 3 (P0): Minimal policy engine on action types (DIFFERENTIATOR)

Goal: Evaluate policies using action semantics, not just paths.

### Tasks

1. Define Policy model v0:

- tenant scope
- rules: match {tool_id?, action_type?, risk>=?} -> decision

2. Implement policy evaluation:

- First-match-wins or priority ordering
- Return decision + rule_id + reason + policy_version

3. Default policy (safe):

- DENY DATA.EXPORT, ACCESS.GRANT, delete operations
- REQUIRE_APPROVAL PAYMENT.REFUND, ACCESS.ROLE_CHANGE
- ALLOW basic reads + ticket create/comment

4. PDP interface:

- internal function now, but shaped like future HTTP PDP

5. Tests:

- action-based policies produce correct decisions

### Acceptance Criteria

- Changing rules changes decisions without touching gateway code
- A request can be denied by action type even if endpoint is different

### Story 4 (P0): Connector abstraction + generic REST connector

Goal: Execute allowed requests via a connector.

### Tasks

1. Connector interface: Execute(ctx, toolConfig, canonicalRequest) -> response
2. Tool config loader (env/file) for:

- base URL
- auth type (bearer/api-key)
- credentials (read from env)

3. Generic REST execution:

- timeouts
- size limits
- safe header forwarding

4. Integration tests with mock tool server

### Acceptance Criteria

Allowed calls reach tool server and responses return to caller

### Story 5 (P0): Approval-required stub + Approval Request record (DIFFERENTIATOR)

Goal: When a decision requires approval, create a record tied to the request hash.

### Tasks

1. Create DB schema for approval_requests (Postgres)

2. On REQUIRE_APPROVAL:

- persist approval_request with request_hash + TTL
- persist ADR with approval_request_id
- return 409 (or 202) with payload including approval_request_id

3. Ensure binding:

- approval_request stores request_hash and cannot be reused for different action

4. Tests:

- approval record created and linked

### Acceptance Criteria

REQUIRE_APPROVAL produces an approval_request_id and ADR record every time

### Story 6 (P0): Action Decision Records (ADR) persistence + evidence preview export (DIFFERENTIATOR)

Goal: Produce governance output early.

### Tasks

1. Create DB schema for action_decisions (ADR)
2. Write ADR on:

- request received
- decision made
- tool executed (if ALLOW) incl response hash

3. Implement evidence export:

- GET /v1/tenants/{tenant_id}/evidence?limit=50
- returns ADR list (JSON) + policy snapshot metadata

4. Tests:

- export returns expected records in order
- redaction: do not return raw payload, only hashes + summaries

### Acceptance Criteria

You can demo “governance” by exporting tenant evidence immediately after actions

### Story 7 (P0): AuthN/AuthZ + tenant scoping (baseline)

Goal: Minimal tenant safety and structure.

### Tasks

1. X-Tenant-Key maps to tenant_id (config-based in Epic 1)
2. Require X-Agent-Id; attach to context
3. Ensure evidence export is tenant-scoped
4. Reject missing/invalid keys with 401, invalid agent with 403
5. Tests for tenant scoping

### Acceptance Criteria

No request proceeds without tenant + agent context

### Story 8 (P0): Observability (logs + metrics)

Goal: Operable and demo-friendly.

### Tasks

1. Structured logs: include request_id, tenant_id, agent_id, tool_id, action_type, decision, latency
2. Metrics:

- decisions_total{decision, action_type}
- gateway_requests_total
- tool_exec_total
- errors_total
- decision_latency_ms histogram

3. Health endpoints

### Acceptance Criteria

You can trace a single action through logs and show basic metrics

## 6) Implementation notes (Go + React later)

- Go router: echo v5
- Logging: echo default logger
- Metrics: Prometheus client
- DB: Postgres (required now because ADR/approval stub are differentiators)
- Migrations: golang-migrate or goose

### Data safety defaults

- Do not store raw tool payloads by default
- Store hashes + redacted summaries only
- Cap request/response sizes (configurable)

## 7) Testing and demo plan (Epic 1)

### Required demo services

- Gateway + Postgres
- Mock PDP (optional; we use internal policy engine in Epic 1)
- Mock internal tool service (refund/export endpoints)
- (Optional) Jira sandbox or mock Jira endpoints

### Demo script (3 actions)

1. Create Jira issue → classified TICKET.CREATE → ALLOW → tool executed → ADR recorded
2. Refund via mock tool → PAYMENT.REFUND → REQUIRE_APPROVAL → returns approval_request_id → ADR recorded
3. Export via mock tool → DATA.EXPORT → DENY → tool NOT executed → ADR recorded

Then:

1. GET /v1/tenants/{tenant}/evidence?limit=50 shows three ADRs

## 8) Timeline (Epic 1 within Week 1–1.5)

- Day 1–2: Gateway canonicalization + auth + connector skeleton
- Day 2–3: Action taxonomy + classifier + internal policy engine
- Day 3–4: ADR persistence + evidence export endpoint
- Day 4–5: Approval stub record + tests + metrics + demo harness

## 9) Open questions (non-blocking, but decide soon)

How do we want tool definitions stored (config file vs DB)?

- Store config in DB preferably.

How detailed should action_summary be (simple now; richer later)?

- It should be rich enough to be deterministic.

Do we want to ship “policy templates” in Epic 1 or Epic 2?

- Basic defaults in Epic 1; templates in Epic 2
