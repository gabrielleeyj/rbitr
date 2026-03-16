# EPIC 11 — MCP & Agent-to-Agent Security Hardening

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

- New file: `internal/policy/base_policy.go` — embedded base Rego module + evaluator
- Modified: `internal/policy/evaluator.go` — two-pass evaluation (base then tenant)
- New file: `internal/policy/base_policy_test.go` — table-driven tests
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

- New file: `internal/auth/session.go` — session token issue/validate
- Modified: `internal/api/public/mcp_handler.go` — issue token on initialize, validate on subsequent calls
- New file: `internal/auth/session_test.go` — token lifecycle tests
- New migration: session tracking table (optional, can use in-memory cache)

---

## Phase 3 — File Access Governance

### Problem

An agent could ask a tool to read arbitrary filesystem paths. The gateway governs the tool call but not the file paths in tool arguments.

### Solution

Three layers of defense:

#### Layer 1 — Argument Path Detection

Extend the classifier to detect file path arguments in tool call payloads. Policy can whitelist/blacklist path patterns per tenant.

```rego
# Deny access to system paths
deny {
    some arg in input.tool_arguments
    is_file_path(arg)
    path_matches(arg, ["/etc/*", "/var/*", "/root/*", "~/*"])
}

# Restrict to tenant upload directory
deny {
    some arg in input.tool_arguments
    is_file_path(arg)
    not startswith(arg, concat("/", ["/data/uploads", input.tenant_id]))
}
```

#### Layer 2 — Tenant-Scoped File Sandbox

Each tenant gets a virtual filesystem root: `/data/tenants/{tenant_id}/`. Path traversal (`../`) is detected and blocked at the gateway.

#### Layer 3 — File Upload API

New endpoint `POST /v1/files` for governed file uploads. Files are assigned IDs; agents reference files by ID, never by path. Gateway resolves IDs to paths only when forwarding to tool servers.

### Implementation

- New file: `internal/classification/file_path.go` — file path detection in arguments
- Modified: `internal/api/public/handlers.go` — inject detected paths into policy input
- New file: `internal/api/public/file_handler.go` — file upload endpoint
- New migration: file metadata table

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

- **Correlation header** (`X-Rbitr-Request-Chain`): Signed JWT containing originating tenant, agent, and decision IDs
- **Cross-tenant policy input**: New field `input.source_tenant_id` lets each tenant's policy allow/deny requests from other tenants. Default: **DENY** all cross-tenant calls
- **ADR linkage**: Agent B's ADR references originating ADR from Agent A
- **Chain depth limit**: Maximum 5 hops to prevent infinite loops

### Implementation

- New file: `internal/auth/provenance.go` — chain token creation/validation
- Modified: `internal/policy/evaluator.go` — inject source_tenant_id into policy input
- Modified: `internal/api/public/handlers.go` — extract and validate chain header
- New migration: cross-tenant allowlist table

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
