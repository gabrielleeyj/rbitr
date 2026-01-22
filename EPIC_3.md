# EPIC 3: End-to-End Approvals (Token-Gated Resubmission, No Payload Storage)

## 0) Purpose (what we are building)

Epic 3 completes the governance loop: **REQUIRE_APPROVAL becomes enforceable control**, not just a logged decision.

**Design choice (locked): Option B**

- On REQUIRE_APPROVAL, gateway returns an **opaque approval token** + `approval_request_id`
- Agent **resubmits the exact same request** with the token after approval
- Gateway verifies:
  - request hash matches the approved request hash
  - token matches (hashed in DB)
  - approval is approved + unexpired + not yet executed
- Gateway executes tool call **exactly once** and records ADR + evidence.

**Non-goal:** Storing raw request/response payloads in DB for replay. We continue “hashes/summaries only.”

---

## 1) Scope and non-scope

### In scope (Epic 3)

1. Approval lifecycle APIs (admin):

- list / detail
- approve / deny (with optional comment)

2. Token-gated resubmission (data plane):

- Generate approval token on REQUIRE_APPROVAL
- Validate token + request hash on resubmission
- Execute once + anti-replay

3. UI changes:

- “Approvals inbox” view + detail page
- Approve / deny actions

4. Evidence + audit updates:

- Evidence export includes approval resolution & execution metadata (no token)
- Admin audit events for approve/deny

5. Tests:

- Unit tests (handlers/store)
- Integration tests (full approve → resubmit → execute once)

### Out of scope (Epic 4+)

- Multi-approver workflow / 2-person rule (we’ll store fields to support later)
- Step-up auth (MFA) for admin actions
- Workflow automation / notifications (Slack/email)
- Cryptographic ledger / hash chaining beyond current ADR hashes

---

## 2) End state (Definition of Done)

A repeatable demo shows:

1. Agent attempts a refund action → policy returns REQUIRE_APPROVAL  
   Gateway responds **409** with:
   - `approval_request_id`
   - `approval_token` (opaque)
     Gateway records ADR (decision=REQUIRE_APPROVAL)

2. Admin approves in UI  
   Approval request transitions PENDING → APPROVED (audited)

3. Agent resubmits the _same request_ including:
   - `X-Approval-Request-Id: <id>`
   - `X-Approval-Token: <token>`
     Gateway validates + executes tool call → returns tool response
     Approval request transitions APPROVED → EXECUTED
     Gateway records ADR (decision=ALLOW) linked to the approval

4. Replay attempts fail:
   - resubmitting with same token again returns 409/403 (“already executed”)
   - resubmitting with a different payload returns 403 (“hash mismatch”)
   - resubmitting after expiry returns 403 (“expired”)

Evidence export for the tenant shows:

- original REQUIRE_APPROVAL decision
- approval resolution fields (approved_by, approved_at, comment)
- executed_at and execution ADR linkage
- still no raw payloads.

---

## 3) System design (flows)

### 3.1 Require-approval response (first attempt)

When policy decision == REQUIRE_APPROVAL:

- Compute `request_hash` as usual
- Create approval_request row:
  - status=PENDING
  - request_hash
  - expires_at = now + approval TTL (from policy constraint or default)
  - approval_token_hash = sha256(token) (store hash only)
  - optionally store `requested_action_summary`, `action_type`, `tool_id`, `risk`, `rule_id`
- Return:

```json
{
  "error": "approval_required",
  "approval_request_id": "ar_123",
  "approval_token": "opaque_random_string",
  "expires_at": "RFC3339",
  "action_type": "PAYMENT.REFUND",
  "risk": "CRITICAL"
}
```

### 3.2 Admin approve/deny

Admin endpoints set:

- status=APPROVED or DENIED
- decided_at, decided_by, comment
- write admin_audit_event

### 3.3 Agent resubmission (execution)

On POST /v1/tools/{tool_id}/call, if headers include approval id + token:

1. Recompute request_hash for the incoming request
2. Load approval_request by id
3. Validate:

- tenant_id matches
- status == APPROVED
- now < expires_at
- approval_token_hash == sha256(token)
- request_hash == approval_request.request_hash
- executed_at is NULL (single-use)

4. Execute tool call
5. Update approval_request:

- status=EXECUTED
- executed_at
- executed_request_id (request_id)
- executed_adr_id (decision_id)

6. Persist ADR for execution (decision=ALLOW), linked to approval_request_id
   **Important**: token is never stored or returned again; only hash stored.

## 4) API surface (Epic 3)

### Data plane (existing endpoint, extended behavior)

- `POST /v1/tools/{tool_id}/call`
  - Add optional headers:
    - `X-Approval-Request-Id`
    - `X-Approval-Token`

### New admin endpoints

Approvals:

- `GET /admin/tenants/{tenant_id}/approvals?status=PENDING|APPROVED|DENIED|EXECUTED&limit=&offset=`
- `GET /admin/tenants/{tenant_id}/approvals/{approval_request_id}`
- `POST /admin/tenants/{tenant_id}/approvals/{approval_request_id}/approve`
  - body: `{ "comment": "optional" }`
- `POST /admin/tenants/{tenant_id}/approvals/{approval_request_id}/deny`
  - body: `{ "comment": "optional" }`

(Optional but recommended)

- `POST /admin/tenants/{tenant_id}/approvals/{approval_request_id}/revoke`
  - transitions APPROVED → DENIED (or REVOKED) before execution

## 5) Data model changes

### 5.1 approval_requests table updates

Assuming you already have an `rbitr.approval_requests` table from EPIC 1, extend it.

Required new columns (minimum viable):

- `status` TEXT NOT NULL -- PENDING, APPROVED, DENIED, EXECUTED, EXPIRED
- `approval_token_hash` TEXT NOT NULL
- `expires_at` TIMESTAMPTZ NOT NULL
- `decided_at` TIMESTAMPTZ NULL
- `decided_by` TEXT NULL -- admin actor_id or display
- `decision_comment` TEXT NULL
- `executed_at` TIMESTAMPTZ NULL
- `executed_request_id` TEXT NULL
- `executed_decision_id` TEXT NULL -- ADR decision_id

Useful denormalized context (helps UI without joining ADR):

- tenant_id (already)
- agent_id
- tool_id
- action_type
- risk
- rule_id
- request_hash (already)
- created_at (already)

Indexes:

- `(tenant_id, status, created_at desc)`
- `(tenant_id, expires_at)`
- `(approval_request_id)` primary key

### 5.2 ADR updates (if needed)

Ensure ADR includes:

- `approval_request_id` (already)
- For execution ADR: link back to approval_request_id
- Evidence export DTO extends to include:
  - approval status fields (safe subset)
  - executed_at, decided_at, decided_by (no token)

### 5.3 Admin audit events

Emit audit actions:

- `APPROVAL.REQUEST.APPROVE`
- `APPROVAL.REQUEST.DENY`
- `APPROVAL.REQUEST.REVOKE` (if you add revoke)

## 6) Stories and tasks breakdown

### Story 1 (P0): Approval token issuance on REQUIRE_APPROVAL

**Goal**: Gateway returns approval token + persists hashed token with approval request.

### Tasks

1. Generate cryptographically secure random token (e.g., 32 bytes base64url)
2. Hash token (sha256) and store approval_token_hash
3. Add/extend DB columns + migration
4. Modify REQUIRE_APPROVAL handler path:

- create approval request with status=PENDING, expires_at
- return 409 payload incl token and id

5. Tests:

- token returned
- token hash stored
- token never appears in evidence export

### Acceptance Criteria

- Any REQUIRE_APPROVAL response includes approval_request_id + token + expires_at
- DB only stores token hash

### Story 2 (P0): Admin approval lifecycle endpoints (list/detail/approve/deny)

**Goal**: Control plane can resolve approval requests.

### Tasks

1. Store methods:

- ListApprovals (filter by status, tenant)
- GetApprovalByID
- ApproveApprovalRequest (with comment)
- DenyApprovalRequest (with comment)

2. Handlers for endpoints listed in Section 4
3. Enforce admin auth scopes:

- list/detail: admin:read
- approve/deny: admin:write

4. Emit admin audit events on approve/deny
5. Tests:

- handler tests for auth, write lock, state transitions
- store tests for transitions

### Acceptance Criteria

- Admin can approve/deny from UI and see state updated immediately
- Every approve/deny emits an audit record

### Story 3 (P0): Token-gated resubmission execution path (single-use, anti-replay)

**Goal**: Agent resubmits exact request and gateway executes once.

### Tasks

1. In public tool-call handler, detect headers:

- X-Approval-Request-Id + X-Approval-Token

2. Load approval_request; validate:

- tenant match
- status APPROVED
- not expired
- token hash matches
- request_hash matches current computed hash
- not executed

3. Execute tool call if valid
4. Update approval_request to status=EXECUTED with executed_at and linkage
5. Persist execution ADR with decision=ALLOW linked to approval_request_id
6. Tests (integration):

- approve then resubmit executes
- replay fails (already executed)
- mismatch payload fails (hash mismatch)
- expired fails

### Acceptance Criteria

- Approved request executes exactly once
- Any mismatch or reuse is blocked with clear error reason
- ADR + evidence reflect approval + execution

### Story 4 (P0): UI “Approvals Inbox” + Detail + actions

**Goal**: Operators can manage approvals without API calls.

### Tasks

1. Add “Approvals” section in UI:

- tabs for PENDING/APPROVED/DENIED/EXECUTED
- pagination & filters (reuse audit patterns)

2. Approval detail page:

- action summary, tool, agent, risk, rule_id, reasons
- request_hash (display), expires_at

3. Approve/Deny actions with comment
4. Toasts + refresh behavior
5. UI tests (smoke) optional; rely on e2e demo script

### Acceptance Criteria

- Approvals can be resolved end-to-end via UI
- Status updates and audit trail visible

### Story 5 (P1): Evidence export and DTO/schema updates for approval resolution

**Goal**: Evidence pack contains proof of approvals (no secrets).

### Tasks

1. Extend evidence export DTO:

- include approval_request_id, approval status, decided_by/at, executed_at
- exclude approval_token always

2. Update JSON schema validation tests and negative-leak tests
3. Add integration test: evidence after approval+execution includes new fields

### Acceptance Criteria

- Evidence export includes approval resolution metadata
- Redaction contract remains enforced

### Story 6 (P1): Expiry management and cleanup

**Goal**: Expired approvals fail correctly and can be cleaned.

### Tasks

1. Enforcement: if now > expires_at → treat as EXPIRED (or fail and keep status)
2. Optional endpoint: admin can mark expired/cleanup
3. Optional periodic cleanup job (manual command for now)
4. Tests for expiry behavior

### Acceptance Criteria

- Expired approvals cannot be executed
- Status is consistent and visible

### Story 7 (P1): Metrics + observability for approvals

**Goal**: Monitor approval system health.

### Tasks

1. Counters:

- approvals_created_total
- approvals_resolved_total{resolution=approved|denied}
- approvals_execute_total{result=success|fail,reason}

2. Logs include:

- approval_request_id, request_id, tenant_id, action_type

3. Dashboard notes (README snippet)

### Acceptance Criteria

- Metrics reflect approval flow; no high-cardinality labels

### 7) Testing plan (must-have)

1. Unit tests:

- Store: approve/deny transitions, executed transitions, invalid transitions
- Handlers: auth, write lock, validation

2. Integration tests:

- Full path:
  - initial call -> REQUIRE_APPROVAL
  - admin approve
  - agent resubmit executes once
- Negative:
  - reuse token fails
  - wrong token fails
  - hash mismatch fails
  - expired fails

3. Contract tests:

- Evidence export schema validation
- Negative-leak: token never appears in export

### 8) Demo script (update)

Extend make demo / scripts/demo.sh to include:

1. trigger approval required
2. call admin approve endpoint
3. resubmit with token
4. fetch evidence export showing approval+execution

### Acceptance Criteria

- One command produces a complete approval loop demo

## 9) Open questions (non-blocking; pick sensible defaults)

- Default TTL for approvals if policy doesn’t specify (recommend 15 minutes)
- Error codes:
  - 409 for approval_required
  - 403 for token mismatch/expired/not approved
  - 409 for already executed (or 410 Gone)
- Whether to allow “revoke” approval before execution (recommended but optional)
