# EPIC 2: React Control Plane + Policy Lifecycle (Boardroom-Ready Productization)

## 0) Purpose (what we are building)

Epic 2 turns the working governance gateway into a **usable control plane**:

- A **React Admin Console** that makes the system operable without direct DB edits/curl guessing
- A **Policy Lifecycle** (versioning, publish/rollback, simulation) so teams can change governance safely
- Clear admin guardrails (write lock, audit trail, safe defaults)

**Outcome:** A first-time user can deploy the stack, open the UI, and in <10 minutes:

- view governed actions (evidence)
- view pending approvals (stub or real list)
- update policy (new version) and publish it
- run a simulation to see what would be allowed/denied
- download an evidence pack preview

> Note: You already have core endpoints from Epic 1. Some of the files you uploaded earlier have expired on my side, so this plan is written against the endpoint set you listed. If you want this Plan.md to perfectly match your exact request/response DTOs, re-upload the handler/DTO files later and I’ll refine the UI contracts.

---

## 1) Scope and non-scope

### In scope (Epic 2)

**React Control Plane**

- Login/auth (admin key based for now)
- Tenant selection
- Evidence viewer (filter/search)
- Policy viewer
- Policy editor (minimal)
- Publish/rollback
- Risk overrides CRUD
- Tool config CRUD (base URL, auth type, enabled)
- Admin write lock toggle
- Basic “evidence export” download flow

**Policy Lifecycle**

- Store multiple policy versions
- Active policy pointer per tenant
- Rollback to previous
- Policy simulation endpoint (evaluate without executing tool calls)

**Audit for admin changes**

- Record who changed what (at least structured logs; DB table preferred)

### Out of scope (Epic 2)

- End-to-end approvals execution (approve → replay tool call) (Epic 3)
- Broad connector marketplace / SDKs (Epic 3/4)
- Multi-tenant SaaS hosted control plane (future)
- Advanced visualizations (graphs, dashboards) beyond basic metrics display

---

## 2) Deliverables (Definition of Done)

1. React app runs locally via docker-compose and points at API
2. Admin auth implemented for UI calls:
   - uses `Authorization: Bearer <admin_key>` or `X-Admin-Key`
3. Screens:
   - Tenants (list/select)
   - Evidence (list/detail, filters)
   - Policy (view versions, view active)
   - Policy Editor (edit + save new version)
   - Publish/Rollback
   - Risk Overrides (list/edit/delete)
   - Tools (list/edit)
   - Admin Settings (write lock toggle)
4. API additions (if missing):
   - List tenants
   - List policy versions
   - Publish policy version
   - Rollback policy version
   - Policy simulation
   - List risk overrides
   - Delete risk override
   - List tools
5. Policy lifecycle works end-to-end:
   - create version → publish → gateway uses it on subsequent tool calls
   - rollback changes behavior deterministically
6. Tests:
   - API handler tests for new endpoints
   - Basic UI component tests optional; e2e smoke script required
7. Demo flow documented:
   - “change policy to require approval for refunds” → publish → run tool call → see evidence updated
   - “rollback policy” → behavior reverts

---

## 3) Architecture (control plane vs data plane)

- **Data plane**: Gateway tool-call endpoint (Epic 1)
- **Control plane**: Admin APIs + React UI (Epic 2)
- **Policy lifecycle**: DB as source-of-truth; gateway reads active version per tenant

Suggested layout:

- `cmd/api` (Echo server: public + admin + control-plane endpoints)
- `ui/` (React app)
- `internal/store` (policy versions + tenant config)
- `internal/policy` (OPA evaluator still invoked by gateway)
- `migrations/` for schema updates

---

## 4) API surface (Epic 2)

### Existing (from Epic 1)

Public:

- `POST /v1/tools/{tool_id}/call`
- `GET /v1/tenants/{tenant_id}/evidence?limit=50`

Admin (first-run today):

- `PUT /admin/tenants/{tenant_id}/config`
- `PUT /admin/tenants/{tenant_id}/tools/{tool_id}`
- `PUT /admin/tenants/{tenant_id}/policy`
- `PUT /admin/tenants/{tenant_id}/risk-overrides/{action_type}`
- `PUT /admin/bootstrap/complete`

### Epic 2 updates (recommended)

> Keep existing endpoints working, but add these for a real UI.

**Tenants**

- `GET /admin/tenants` (list)
- `GET /admin/tenants/{tenant_id}` (details)

**Evidence**

- `GET /admin/tenants/{tenant_id}/evidence?limit=50&decision=&action_type=&risk=&since=`
  - (or reuse public endpoint with admin auth; admin UI can call the same endpoint if it’s safe)

**Policy lifecycle**

- `GET /admin/tenants/{tenant_id}/policies` (list versions + metadata)
- `GET /admin/tenants/{tenant_id}/policies/{policy_version}` (fetch rego)
- `POST /admin/tenants/{tenant_id}/policies` (create new version)
- `PUT /admin/tenants/{tenant_id}/policies/{policy_version}/publish`
- `PUT /admin/tenants/{tenant_id}/policies/rollback` (to previous or specified)

**Policy simulation**

- `POST /admin/tenants/{tenant_id}/policies/simulate`
  - input: same policyInput as gateway (no tool execution)
  - output: DecisionV2 + validation results

**Risk overrides**

- `GET /admin/tenants/{tenant_id}/risk-overrides`
- `PUT /admin/tenants/{tenant_id}/risk-overrides/{action_type}`
- `DELETE /admin/tenants/{tenant_id}/risk-overrides/{action_type}`

**Tools**

- `GET /admin/tenants/{tenant_id}/tools`
- `PUT /admin/tenants/{tenant_id}/tools/{tool_id}`
- `POST /admin/tenants/{tenant_id}/tools` (optional if tools are dynamic)

**Admin settings**

- `GET /admin/settings`
- `PUT /admin/settings/write-lock` (toggle)
  - (if you added admin_write_lock in DB)

---

## 5) Data model changes (Epic 2)

### 5.1 Policy versions

Add table (or evolve existing):

- `rbitr.policies`
  - tenant_id
  - policy_version (unique)
  - rego_module (text)
  - created_at
  - created_by (admin_key_id or string)
  - notes (optional)

Add pointer:

- `rbitr.tenant_config` or `rbitr.tenants.active_policy_version`

### 5.2 Admin audit (minimum viable)

Either:

- `rbitr.admin_audit_events` (preferred)
  or
- structured logs only (acceptable MVP)

Fields:

- tenant_id
- actor (admin key id)
- action (POLICY_CREATE, POLICY_PUBLISH, TOOL_UPDATE, RISK_OVERRIDE_UPDATE, WRITE_LOCK_TOGGLE)
- before/after (jsonb)
- timestamp

---

## 6) Stories and tasks breakdown

## Story 1 (P0): React app scaffold + admin auth + routing

**Goal:** A usable shell with authenticated API calls.

### Tasks

1. Create React app structure (Vite recommended)
2. Auth mechanism:
   - Store admin key locally (dev only) OR simple login form storing admin key in memory
3. API client:
   - typed fetch wrapper
   - inject admin auth header on requests
4. Routing + layout:
   - left nav: Tenants / Evidence / Policies / Risk Overrides / Tools / Settings
5. Basic e2e smoke: UI loads, fetches tenants

**Acceptance Criteria**

- UI can authenticate and call at least one admin endpoint successfully

---

## Story 2 (P0): Tenant list + selection

**Goal:** Operator can select a tenant and manage it.

### Tasks

1. Add `GET /admin/tenants` endpoint if missing
2. UI: Tenants page lists tenants; selecting tenant sets global context
3. Display tenant metadata (active policy version, tool count)

**Acceptance Criteria**

- Selecting a tenant updates subsequent pages (Evidence, Policies, etc.)

---

## Story 3 (P0): Evidence viewer (governance proof UX)

**Goal:** Make value visible immediately.

### Tasks

1. Evidence list:
   - table: time, action_type, decision, risk, tool_id, rule_id
2. Filters:
   - decision (ALLOW/DENY/REQUIRE_APPROVAL)
   - action_type
   - risk
   - time window (last N minutes/hours)
3. Evidence detail drawer:
   - show DecisionV2 fields (rule, reasons, constraints summary)
   - show hashes (request/response) + approval_request_id if present
4. Download:
   - “Download JSON evidence” (calls evidence endpoint)

**Acceptance Criteria**

- Operator can find a denied export and see the reason/rule in <30 seconds

---

## Story 4 (P0): Policy viewer + version list

**Goal:** See what policy is active and what versions exist.

### Tasks

1. Add `GET /admin/tenants/{tenant_id}/policies` endpoint if missing
2. UI shows:
   - active policy version highlighted
   - list of versions with created_at/created_by/notes
3. View policy rego text (read-only)

**Acceptance Criteria**

- Operator can view active policy contents in UI

---

## Story 5 (P0): Create policy version + publish/rollback (policy lifecycle)

**Goal:** Safe policy changes.

### Tasks

1. Backend:
   - `POST /admin/tenants/{tenant_id}/policies` to create version
   - `PUT .../publish` to set active policy version
   - rollback endpoint (previous or specified)
2. UI:
   - Policy editor: textarea + notes
   - Validate rego via OPA compile check on save (server-side)
   - Publish button
   - Rollback button

**Acceptance Criteria**

- After publishing, gateway evaluations use new policy (validated via integration test)

---

## Story 6 (P1): Policy simulation endpoint + UI

**Goal:** Preview policy behavior without executing tool calls.

### Tasks

1. Backend: `POST /admin/tenants/{tenant_id}/policies/simulate`
   - input: policyInput-like object
   - output: DecisionV2 + validation info
2. UI: “Simulate” panel:
   - select action_type/tool/risk, optional attributes
   - run simulate, show decision output

**Acceptance Criteria**

- Operator can simulate “DATA.EXPORT” and see DENY with reasons

---

## Story 7 (P0): Risk overrides UI + CRUD

**Goal:** Operational control without policy edits.

### Tasks

1. Backend:
   - `GET risk-overrides`, `PUT`, `DELETE`
2. UI:
   - list overrides by action_type → risk
   - create/update override
   - delete override

**Acceptance Criteria**

- Override takes effect immediately on subsequent tool calls (verify via test)

---

## Story 8 (P1): Tools management UI + update

**Goal:** Configure connectors without bootstrap-only workflow.

### Tasks

1. Backend:
   - `GET /tools`, `PUT /tools/{tool_id}`
2. UI:
   - list tools: tool_id, base_url, enabled, auth type (no secrets shown)
   - update base_url/enabled
3. Validation:
   - base_url is valid URL
   - tool_id format

**Acceptance Criteria**

- Tool base URL can be updated and gateway uses it (integration test with mock tool)

---

## Story 9 (P1): Admin write lock toggle + guardrails

**Goal:** Prevent accidental changes in demos/prod.

### Tasks

1. Backend:
   - add/read `admin_write_lock`
   - enforce lock on mutating admin endpoints
2. UI:
   - settings page toggle
   - warning banner when locked

**Acceptance Criteria**

- When locked, write endpoints return 423 or 403 with clear reason

---

## Story 10 (P1): Admin audit events (minimum)

**Goal:** governance for the control plane itself.

### Tasks

1. Add audit logging for:
   - policy create/publish/rollback
   - tool updates
   - risk override changes
   - write lock toggle
2. Persist to DB or structured logs
3. (Optional) UI shows recent admin changes

**Acceptance Criteria**

- Every config change is attributable (who/when/what)

---

## 7) Testing plan

- Backend handler tests for new endpoints (tenants list, policy lifecycle, simulate, list/delete overrides)
- Integration test:
  - create policy version → publish → tool call decision changes → evidence shows new rule_id
- UI smoke test:
  - load tenants, view evidence, view policy, publish policy (optional)
- Maintain evidence export schema validation + redaction tests (must stay green)

---

## 8) Timeline (2-week plan; extend to 3 if solo dev)

### Week 1

- Story 1–4 (UI scaffold, tenant selection, evidence viewer, policy viewer)
- Backend endpoints needed for list/read operations
- Basic demo: view evidence + view policy

### Week 2

- Story 5–7 (policy create/publish/rollback, simulate, overrides UI)
- Add admin write lock if not already
- Demo: change policy → publish → behavior changes → evidence shows it

(Week 3 optional)

- Tools UI + admin audit UI polish

---

## 9) Open questions (decide early)

- Admin auth mechanism: admin key only now (OK) vs JWT (later)
- Do we keep bootstrap_complete semantics at all, or replace with admin_write_lock?
- Policy version naming convention: timestamp-based vs semantic

---
