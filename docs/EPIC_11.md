# EPIC 11 — MCP & Agent-to-Agent Security Hardening

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** Mandatory Base Policy | **DONE** | 2026-03-15 |
| **2** Ephemeral Session Tokens | **DONE** | 2026-03-15 |
| **3** File Access Governance | **DONE** | 2026-03-16 |
| **4** Cross-Tenant Provenance Chain | **DONE** | 2026-03-16 |
| **5** mTLS Client Certificates | Planned | — |

## Summary

Epic 11 enhances policy defaults and security controls for MCP and agent-to-agent communications. It addresses five critical gaps: (1) no mandatory base policy for high-risk actions, (2) static tenant keys vulnerable to spoofing, (3) no cross-tenant request governance, (4) no file access sandboxing, and (5) no ephemeral session key issuance.

Date scoped: 2026-03-13

---

## Problem Statement

When multiple agents connected via different tenant IDs communicate with each other through the gateway, several security risks emerge:

1. **No mandatory safety net:** A new tenant with no custom policy gets the default policy, but there is no system-level base policy that cannot be overridden. A tenant could craft a permissive policy that allows destructive actions without approval.

2. **Static key spoofing:** Tenant keys are long-lived HMAC-SHA256 hashed secrets. If compromised, they remain valid until manually revoked. There is no mechanism to automatically issue short-lived session tokens on agent connect.

3. **Cross-tenant blind spots:** If Agent A (tenant-1) calls a tool that triggers Agent B (tenant-2), the gateway treats these as independent requests with no awareness of the provenance chain. Neither tenant's policy governs the full chain.

4. **File system exfiltration:** If an agent uses a tool that accesses the filesystem, the gateway only governs the tool call, not what the tool does internally. An agent could request "read /etc/passwd" or scan directories for credentials.

---

## Phase 1 — Mandatory Base Policy Layer

### Problem

A tenant could define a fully permissive Rego policy that ALLOWs all actions including `DATA.DELETE`, `ACCESS.GRANT`, and `DATA.BULK_EXPORT`. There is no system-level guardrail that cannot be overridden.

### Solution

Introduce a **base policy** that is always evaluated **before** the tenant policy. If the base policy returns `DENY`, the tenant policy is never evaluated — the decision is final. If the base policy returns `REQUIRE_APPROVAL`, the tenant policy cannot downgrade it to `ALLOW`.

```
Evaluation Chain:
━━━━━━━━━━━━━━━━

1. Evaluate base policy with same input
2. If base → DENY → return DENY (tenant policy skipped)
3. If base → REQUIRE_APPROVAL → evaluate tenant policy
   a. If tenant → DENY → return DENY
   b. If tenant → ALLOW → return REQUIRE_APPROVAL (base wins)
   c. If tenant → REQUIRE_APPROVAL → return REQUIRE_APPROVAL
4. If base → ALLOW → evaluate tenant policy → return tenant decision
```

### Base Policy Rules

```rego
package rbitr.base_policy

import rego.v1

# CRITICAL risk always requires approval (cannot be overridden to ALLOW)
require_approval_critical {
    input.action_risk == "CRITICAL"
}

# Destructive actions at HIGH/CRITICAL risk require approval
require_approval_destructive {
    input.action_type in {"DATA.DELETE", "CRM.DELETE", "DATA.BULK_EXPORT"}
    input.action_risk in {"HIGH", "CRITICAL"}
}

# Identity/access actions always require approval
require_approval_access {
    input.action_type in {"ACCESS.GRANT", "ACCESS.ROLE_CHANGE"}
}

# Bulk operations require approval
require_approval_bulk {
    input.action_type in {"DATA.BULK_EXPORT", "DATA.EXPORT"}
}
```

### Implementation

- `internal/policy/base_policy.go` — embedded base Rego module + evaluator
- `internal/policy/evaluator.go` — two-pass evaluation (base then tenant)
- `internal/policy/base_policy_test.go` — table-driven tests for all action type / risk combinations
- ADR records base policy decision in `tags` field: `["base_policy:REQUIRE_APPROVAL"]`

### Acceptance Criteria

- Base policy DENY cannot be overridden by any tenant policy
- Base policy REQUIRE_APPROVAL cannot be downgraded to ALLOW by tenant policy
- Base policy ALLOW defers to tenant policy decision
- All CRITICAL risk actions require approval regardless of tenant policy
- All ACCESS.* actions require approval regardless of tenant policy
- ADR audit trail includes base policy decision tag
- No regression on existing policy evaluation tests

---

## Phase 2 — Ephemeral Session Tokens

### Problem

Tenant keys are static. If a key is leaked, it stays valid until manual revocation. Agents cannot be bound to a session or IP.

### Solution

Issue short-lived session tokens during the MCP `initialize` handshake:

```
Agent Connect Flow:
━━━━━━━━━━━━━━━━━━

1. Agent presents long-lived tenant key via initialize
2. Gateway validates tenant key
3. Gateway issues session token (JWT, 15-min TTL)
4. All subsequent requests use session token
5. Session token bound to: tenant_id, agent_id, source_ip
6. Token expires → agent must re-authenticate
```

### Session Token Structure

```json
{
  "tid": "tenant_1",
  "aid": "agent_sales_bot",
  "sid": "sess_abc123",
  "iat": "2026-03-13T10:00:00Z",
  "exp": "2026-03-13T10:15:00Z",
  "sip": "10.0.1.42"
}
```

### Security Properties

- **IP binding:** Token rejected if source IP changes
- **Anti-replay:** Session ID stored in gateway cache; nonce in handshake
- **Capability scoping:** Session token can carry reduced permissions
- **Rotation:** Gateway HMAC signing key rotates on configurable schedule

### Implementation

- `internal/auth/session.go` — `SessionManager` with HMAC-SHA256 signed tokens, in-memory TTL cache for revocation
- `internal/auth/session_test.go` — token lifecycle, IP binding, expiry, revocation tests
- `internal/api/public/mcp_handler.go` — issues session token on `initialize`, validates on subsequent `tools/call`
- `internal/cache/ttl_cache.go` — generic TTL cache (used for session revocation tracking)
- Feature flag: `RBTR_FEATURE_SESSION_TOKENS=true`
- Session TTL: `RBTR_SESSION_TOKEN_TTL_SECONDS` (default `900` / 15 minutes)

---

## Phase 3 — File Access Governance

### Problem

An agent could ask a tool to read arbitrary filesystem paths. The gateway governs the tool call but not the file paths in tool arguments.

### Solution

Two layers of defense:

#### Layer 1 — Argument Path Detection

The classifier recursively walks tool call arguments (JSON objects, arrays, strings) to detect file path patterns. Detected paths are validated against the tenant's sandbox before the request reaches the policy engine.

Detection heuristics:
- Absolute paths starting with `/`
- Relative paths containing `/` separators
- Windows-style paths (`C:\`, `D:\`)
- Home directory references (`~/`)

#### Layer 2 — Tenant-Scoped File Sandbox

Each tenant gets a virtual filesystem root: `/data/tenants/{tenant_id}/`. Path traversal (`../`) is detected and blocked at the gateway by inspecting raw path segments (not `filepath.Clean`'d, which resolves `..` away).

Enforcement:
- Path traversal (`..` in any segment) → blocked immediately
- Path outside tenant sandbox → blocked
- Path within sandbox → allowed

Feature flag: `RBTR_FEATURE_FILE_GOVERNANCE=true`

### Implementation

- `internal/classification/file_path.go` — `DetectFilePaths`, `IsFilePath`, `ContainsTraversal`, `ValidateSandbox`, `SandboxRoot`
- `internal/classification/file_path_test.go` — table-driven tests
- `internal/api/public/file_governance.go` — `checkFileAccess` helper wired into both REST and MCP handlers
- `internal/api/public/file_governance_test.go` — handler-level tests
- `internal/mcp/types.go` — `ErrorFileAccessDenied = -32006`
- No migration needed (enforcement is stateless, based on argument inspection)

---

## Phase 4 — Cross-Tenant Request Provenance Chain

### Problem

When Agent A (tenant-1) triggers a tool call that causes Agent B (tenant-2) to act, the gateway sees two independent requests with no awareness of the chain.

### Solution

Introduce a signed request provenance chain:

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Agent A     │────>│  Gateway     │────>│  Agent B     │
│  (tenant-1)  │     │  (validates  │     │  (tenant-2)  │
│              │     │   chain)     │     │              │
└──────────────┘     └──────────────┘     └──────────────┘
                          │
                    Checks BOTH tenant-1
                    AND tenant-2 policies
```

### Key Concepts

- **Provenance header** (`X-Provenance-Chain`): HMAC-SHA256 signed token containing source tenant ID, source decision ID, chain depth, and expiry (30s TTL)
- **Cross-tenant policy input**: New fields `input.source_tenant_id` and `input.chain_depth` let each tenant's policy allow/deny requests from other tenants
- **ADR linkage**: Downstream ADR references the originating decision via `source_decision_id`
- **Chain depth limit**: Configurable maximum (default 5) to prevent infinite loops

Feature flag: `RBTR_FEATURE_CROSS_TENANT_CHAIN=true`
Max chain depth: `RBTR_MAX_CHAIN_DEPTH` (default `5`)

### Implementation

- `internal/auth/provenance.go` — `ProvenanceManager` with HMAC-signed token issue/validate (reuses session.go pattern)
- `internal/auth/provenance_test.go` — 8 test cases covering token lifecycle, depth limits, expiry, and signature validation
- `internal/api/public/provenance.go` — `extractProvenance` helper and `injectProvenanceInput` for policy enrichment
- `internal/api/public/feature_flags.go` — `featureCrossTenantChainEnabled`
- `internal/api/public/handlers.go` — provenance extraction + policy injection in REST handler, `SourceDecisionID` in ADRs
- `internal/api/public/mcp_handler.go` — provenance extraction + policy injection in MCP handler, `SourceDecisionID` in ADRs
- `internal/models/models.go` — `SourceDecisionID` field on `ActionDecisionRecord` and `ActionDecisionExport`
- `internal/store/store.go` — `InsertADR` includes `source_decision_id` column
- `cmd/gateway/main.go` — `initProvenanceManager` wired to Dependencies
- `migrations/00030_cross_tenant_provenance.sql` — adds `source_decision_id` column with partial index

---

## Phase 5 — mTLS Client Certificates & Token Binding

### Problem

Even with session tokens, a compromised token can be used from the same IP. For maximum security, agent identity should be bound to a TLS client certificate.

### Solution

- Agents present TLS client certificates on connect
- Certificate fingerprint is bound to the session token
- Gateway validates both the token and the certificate on every request
- Certificate management via admin API (issue, revoke, rotate)

### Implementation

- TLS configuration changes in gateway startup
- Client certificate verification middleware
- Admin API endpoints for certificate lifecycle
- Integration with existing tenant key model

---

## Implementation Priority

| Phase | Enhancement | Effort | Security Impact |
|-------|------------|--------|-----------------|
| **1** | Mandatory base policy layer | Medium | Prevents catastrophic ungovemed actions |
| **2** | Ephemeral session tokens | Medium | Eliminates static key spoofing |
| **3** | File access governance | Medium | Prevents filesystem exfiltration |
| **4** | Cross-tenant provenance chain | High | Full agent-to-agent traceability |
| **5** | mTLS client certificates | High | Hardware-grade identity binding |

---

## Definition of Done

- Phase 1: Base policy evaluates before tenant policy; DENY/REQUIRE_APPROVAL cannot be overridden
- Phase 2: Session tokens issued on initialize with TTL, IP binding, and anti-replay
- Phase 3: File paths detected in arguments; sandbox enforced per tenant
- Phase 4: Cross-tenant request chain tracked with full ADR linkage
- Phase 5: mTLS client certs bound to sessions

## Non-Goals for Epic 11

- Custom base policy per tenant (base policy is system-wide and immutable)
- Agent identity federation (e.g., SAML/OIDC for agents — agents authenticate via tenant keys)
- End-to-end encryption of tool payloads (transport security via TLS is sufficient)
- Real-time agent behavior anomaly detection (separate ML/observability epic)
