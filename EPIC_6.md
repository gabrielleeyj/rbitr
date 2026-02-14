# EPIC 6 - MCP Compatibility (Minimal, Safe)

## Goal

Add **minimal MCP support** so MCP clients can route traffic through rbitr with **low surprises** and without changing the core governance model.

We will support:

- **MCP Streamable HTTP transport** (remote standard)
- Core MCP methods:
  - `tools/list` (tool discovery)
  - `tools/call` (governed execution)
- **Pass-through JSON-RPC** for other MCP methods (resources/prompts later)

We will keep rbitr’s internal governance unchanged:
MCP `tools/call` → canonical request → classify → OPA policy eval → decision → (approval gate) → forward upstream → return.

---

## Non-Goals (Explicit)

- No stdio transport in this epic (would require local sidecar)
- No interactive Slack approvals / callbacks
- No MCP resources/prompts semantics (pass-through only)
- No attempt to fully implement every MCP spec feature
- No breaking changes to existing `/v1/tools/{tool_id}/call`

---

## Architecture Summary

### Topology (Terminate + Forward)

rbitr becomes:

- an **MCP server** to the agent/client
- an **MCP client** to upstream MCP servers (or MCP-compatible endpoints)
- MCP Client → rbitr (MCP Server, Streamable HTTP)
  → governance (policy/approval/evidence)
  → upstream MCP server(s) (Streamable HTTP)

### Key Design Principles

- **Protocol correctness first**: preserve JSON-RPC ids, error codes, and ordering.
- **No secret leaks**: no tool payloads stored or logged beyond existing redaction/hashing.
- **Governance remains canonical**: MCP is just another frontend.
- **Feature-gated**: MCP endpoints behind config flag per tenant/tool.

---

## Scope Breakdown

### Story 1 — MCP Transport Front Door (Streamable HTTP)

**Description**
Add an HTTP endpoint that speaks MCP Streamable HTTP and handles JSON-RPC messages.

**Tasks**

- Add new route group: `POST /mcp/{tenant_id}` or `POST /mcp` (choose final path)
- Implement minimal MCP session handling:
  - request correlation id
  - timeouts
  - max body size
- Implement JSON-RPC envelope parsing and validation:
  - `jsonrpc == "2.0"`
  - `id` can be string/number/null (notifications)
  - `method` required
- Implement response writer:
  - correct JSON-RPC response
  - error handling mapping

**Acceptance Criteria**

- MCP client can send valid JSON-RPC over Streamable HTTP and receive correct JSON-RPC responses
- Invalid JSON-RPC requests return proper JSON-RPC error objects (not raw HTTP errors)
- Metrics and structured logs include `tenant_id`, `mcp_method`, `request_id` (no secrets)

---

### Story 2 — `tools/list` (Tool Discovery)

**Description**
Return a list of tools in MCP format. Source of truth is rbitr’s existing tool registry.

**Tasks**

- Define MCP Tool mapping:
  - `name`: stable tool name (or `tool_id`)
  - `description`: tool description (docstring)
  - `inputSchema`: JSON Schema (from tool registry)
- Add tool metadata fields if missing:
  - `description`
  - `input_schema_json`
  - `version` (optional)
- Implement per-tenant filtering:
  - only return tools enabled for tenant
- Add admin UI support (optional in this epic):
  - edit tool description and schema
  - validate JSON schema

**Acceptance Criteria**

- `tools/list` returns a stable list compatible with MCP clients
- Tools include meaningful descriptions + schemas (non-empty for demo tools)
- Unit tests for list output shape and schema validation

---

### Story 3 — `tools/call` (Governed Execution)

**Description**
Intercept MCP tool execution requests, run the existing governance flow, and forward to upstream.

**Tasks**

- Parse MCP `tools/call` payload:
  - tool name
  - arguments (JSON object)
  - optional metadata
- Map MCP call to rbitr canonical request:
  - `tool_id` resolved from MCP tool name
  - `input` = canonical JSON
  - compute hashes and classification
- Invoke existing policy + approval machinery:
  - ALLOW → forward upstream and return response
  - DENY → return JSON-RPC error (with safe message)
  - REQUIRE_APPROVAL → return JSON-RPC error conveying approval required (no payload leak)
- Evidence persistence:
  - Store ADR as you do today
  - Ensure evidence DTO whitelist still holds for MCP traffic

**Acceptance Criteria**

- `tools/call` ALLOW path returns tool output successfully
- DENY returns a JSON-RPC error with deterministic safe structure
- REQUIRE_APPROVAL returns a JSON-RPC error that includes:
  - `approval_required: true`
  - `approval_request_id`
  - `expires_at`
  - (optional) `ui_url` for operators
- Approval resubmission model remains unchanged:
  - For MCP, the resubmission must include approval token in a defined place:
    - either `arguments._rbitr_approval_token`
    - or MCP metadata extension field
  - Token validated and executed exactly once

---

### Story 4 — Upstream MCP Forwarder

**Description**
Forward allowed calls to upstream MCP servers over Streamable HTTP.

**Tasks**

- Extend tool config to include MCP upstream URL:
  - e.g., `tool.transport = "mcp_streamable_http"`
  - `tool.endpoint = https://.../mcp`
- Implement MCP client:
  - forward JSON-RPC request with same `id`
  - handle upstream errors and relay safely
  - set timeouts and retry policy (no retries for non-idempotent calls)
- Add circuit breaker / basic failure handling:
  - upstream timeout counts as tool error
  - do not leak upstream body in logs

**Acceptance Criteria**

- Allowed calls are forwarded upstream and responses returned unchanged (except optional wrapping)
- Timeouts and upstream failures return consistent JSON-RPC errors
- Metrics for upstream latency and failures per tool

---

### Story 5 — Pass-through for Non-Core MCP Methods

**Description**
For MCP methods other than `tools/list` and `tools/call`, forward to upstream without governance.

**Tasks**

- Implement allowlist/denylist of methods:
  - govern only the two core methods
  - forward everything else
- Add safety guardrails:
  - max payload size
  - optional blocklist for obviously dangerous methods (future)

**Acceptance Criteria**

- Unknown/unsupported methods do not break clients:
  - either pass-through if upstream exists
  - or return JSON-RPC "method not found" if no upstream
- No policy evaluation is performed for pass-through methods

---

## API Surface (Proposed)

### New Public Endpoint

- `POST /v/1mcp/{tenant_id}` — MCP Streamable HTTP endpoint

### Existing Endpoint Remains

- `POST /v1/tools/{tool_id}/call` — existing API mode remains stable

---

## Data Model Changes (Minimal)

Add to `rbitr.tools` (or tool config table):

- `transport` enum: `http_api` (default), `mcp_streamable_http`
- `mcp_upstream_url` (if transport is MCP)
- `description` (docstring)
- `input_schema_json` (JSON Schema)

Add admin UI fields to edit these (optional this epic).

---

## Error Mapping (MCP / JSON-RPC)

Define a stable error object for governance decisions:

### Approval required

- JSON-RPC error:
  - code: `-32001` (app-specific)
  - message: "approval required"
  - data:
    - `approval_required: true`
    - `approval_request_id`
    - `expires_at`
    - `ui_url` (optional)

### Deny

- code: `-32003`
- message: "denied by policy"
- data: safe reasons (rule id, risk), no raw payload

### Policy invalid

- code: `-32004`
- message: "policy evaluation error"
- data: safe reason code only

---

## Metrics

Add:

- `mcp_requests_total{method,tenant_id,result}`
- `mcp_tool_calls_total{tool_id,decision}`
- `mcp_upstream_latency_ms{tool_id}`
- `mcp_errors_total{reason}`

(Keep labels low cardinality; avoid user-provided ids in labels.)

---

## Security / Safety Requirements

- No secrets or raw payloads in logs, audit, evidence
- Request body size limits
- Timeouts for upstream forwarding
- Approval tokens never logged; only hashed comparisons
- Preserve existing redaction and DTO whitelist guarantees

---

## Testing Plan

- Unit tests:
  - JSON-RPC parser/validator
  - tools/list output shape
  - tools/call mapping to canonical request
  - error mapping for approval/deny/policy invalid
- Integration tests:
  - MCP client → rbitr → mock upstream MCP server
  - Allow path returns upstream response
  - Approval required path returns approval_request_id + expiry
  - Approve + resubmit executes exactly once
  - Pass-through method forwards (smoke test)
- Negative tests:
  - invalid JSON-RPC
  - oversized request body
  - upstream timeout
  - token mismatch / expired

---

## Acceptance Criteria (Epic-Level)

1. An MCP client can connect via Streamable HTTP and call:
   - `tools/list` successfully
   - `tools/call` successfully
2. rbitr enforces governance decisions for `tools/call`:
   - ALLOW forwards to upstream and returns result
   - DENY returns deterministic JSON-RPC error
   - REQUIRE_APPROVAL returns approval metadata and blocks execution until approved
3. Approval resubmission works for MCP traffic with replay protection and exact-once semantics.
4. Non-core methods are pass-through and do not break clients.
5. No secret or raw payload leakage; existing evidence export guarantees remain valid.

---

## Follow-ups (Next Epics)

- stdio transport via local sidecar
- MCP resources/prompts governance
- Better tool discovery UX (OpenAPI import → schema)
- Interactive Slack approvals / callbacks

---

## Additional Answers

- What is the stable mapping between MCP “tool name” and rbitr tool_id?
  Use `tool_id` as the MCP tool name for stability. Display a friendly label separately.

- Where does the approval token live on MCP resubmission?
  Recommendation (minimal + explicit): allow an extension field in arguments:

`arguments._rbitr_approval_token = "..."`

It’s simple, and you can later support a more standard metadata envelope if MCP formalizes one.

Below are exact JSON-RPC examples you can lift straight into docs + integration tests for MCP Streamable HTTP. I’ll show:

1. tools/list
2. tools/call (ALLOW)
3. tools/call (REQUIRE_APPROVAL)
4. tools/call (DENY)
5. tools/call resubmission with approval token (EXECUTE)
6. Common JSON-RPC errors (invalid request / method not found)

I’ll assume:

- rbitr MCP endpoint: POST /v1/mcp/{tenant_id}
- MCP tool name maps 1:1 to your tool_id (recommended)
- approval token is passed as `arguments._rbitr_approval_token`
- Your app-specific JSON-RPC error codes:
  - 32001 approval required
  - 32003 denied by policy
  - 32004 policy evaluation / invalid output

## 0. Transport shape (Streamable HTTP)

Each JSON-RPC message is sent as a normal HTTP POST with JSON body.

### HTTP request

```
POST /mcp/t_demo HTTP/1.1
Content-Type: application/json

{ ...json-rpc... }
```

### HTTP response

```
HTTP/1.1 200 OK
Content-Type: application/json

{ ...json-rpc response... }
```

## 1. tools/list

### Request

```
{
"jsonrpc": "2.0",
"id": 1,
"method": "tools/list",
"params": {}
}
```

### Response (example)

```
{
"jsonrpc": "2.0",
"id": 1,
"result": {
"tools": [
{
"name": "jira",
"description": "Create and manage Jira issues. Use this to create tickets, comment, transition issues, and search.",
"inputSchema": {
"type": "object",
"additionalProperties": false,
"properties": {
"action": {
"type": "string",
"enum": ["issue_create", "issue_comment", "issue_transition", "issue_search"]
},
"projectKey": { "type": "string" },
"issueType": { "type": "string" },
"summary": { "type": "string" },
"description": { "type": "string" },
"issueKey": { "type": "string" },
"comment": { "type": "string" },
"transitionId": { "type": "string" },
"jql": { "type": "string" }
},
"required": ["action"]
}
},
{
"name": "mock_internal",
"description": "Internal demo tool for testing governed actions.",
"inputSchema": {
"type": "object",
"additionalProperties": true
}
}
]
}
}
```

## 2. tools/call — ALLOW path

### Request (agent calls Jira to create issue)

```
{
"jsonrpc": "2.0",
"id": 2,
"method": "tools/call",
"params": {
"name": "jira",
"arguments": {
"action": "issue_create",
"projectKey": "RBTR",
"issueType": "Task",
"summary": "Investigate customer report",
"description": "Customer reports intermittent 500s. Please investigate."
}
}
}
```

### Response (proxy returns tool output)

```
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [{
      "type": "json",
      "json": {
        "issueKey": "RBTR-123",
        "url": "https://jira.example.com/browse/RBTR-123"
        }
      }]
  }
}
```

You can also include a lightweight governance context in result if you want, but minimal MCP mode can omit it to avoid spec drift. If you do include, put it behind an extension key like \_rbitr.

## 3. tools/call — REQUIRE_APPROVAL path

### Request (high-risk action triggers approval)

```
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "jira",
    "arguments": {
    "action": "issue_transition",
    "issueKey": "RBTR-123",
    "transitionId": "31"
    }
  }
}
```

### Response (JSON-RPC error with approval metadata)

```
{
  "jsonrpc": "2.0",
  "id": 3,
  "error": {
    "code": -32001,
    "message": "approval required",
    "data": {
      "approval_required": true,
      "approval_request_id": "apr_01J9Q7W2H8K8X9V1Y2Z3A4B5C6",
      "expires_at": "2026-02-13T10:05:00Z",
      "ui_url": "https://app.rbitr.example/tenants/t_demo/approvals/apr_01J9Q7W2H8K8X9V1Y2Z3A4B5C6",
      "policy_version": "p_v12",
      "rule_id": "jira.transition.requires_approval",
      "risk": "HIGH",
      "reasons": [
        "Issue transitions can change customer-visible state",
        "Action classified as HIGH risk by policy"
      ]
    }
  }
}
```

Important: do not include raw payloads or secrets in data.

## 4. tools/call — DENY path

### Request (denied action)

```
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "jira",
    "arguments": {
      "action": "issue_create",
      "projectKey": "FIN",
      "issueType": "Bug",
      "summary": "Create ticket in restricted project",
      "description": "Attempting to write into restricted project."
    }
  }
}
```

### Response (JSON-RPC error: denied)

```
{
  "jsonrpc": "2.0",
  "id": 4,
  "error": {
    "code": -32003,
    "message": "denied by policy",
    "data": {
      "denied": true,
      "policy_version": "p_v12",
      "rule_id": "jira.project.restricted.deny",
      "risk": "HIGH",
      "reasons": [
        "Project FIN is restricted",
        "Writes are not allowed for this tenant"
      ],
      "tags": ["policy", "deny"]
    }
  }
}
```

## 5. tools/call — Resubmission after approval (EXECUTE)

After approval in UI, agent retries the same call, adding \_rbitr_approval_token.

### Request (resubmit with token)

```
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
    "params": {
    "name": "jira",
    "arguments": {
      "action": "issue_transition",
      "issueKey": "RBTR-123",
      "transitionId": "31",
      "_rbitr_approval_token": "apt_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    }
  }
}
```

### Response (successful execution)

```
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "content": [
      {
      "type": "json",
        "json": {
          "ok": true,
          "status": "transitioned",
          "issueKey": "RBTR-123"
        }
      }
    ]
  }
}
```

### Resubmit again (replay) → blocked

```
{
  "jsonrpc": "2.0",
  "id": 6,
  "error": {
    "code": -32003,
    "message": "approval token invalid",
    "data": {
      "reason": "already_executed",
      "approval_request_id": "apr_01J9Q7W2H8K8X9V1Y2Z3A4B5C6"
    }
  }
}
```

If you prefer 409/410 semantics, that’s HTTP-land. For MCP/JSON-RPC, keep it in the JSON-RPC error object with a reason code.

## 6. Common JSON-RPC errors (protocol-level)

### Invalid request (missing jsonrpc/id/method)

```
{
  "jsonrpc": "2.0",
  "id": null,
  "error": {
    "code": -32600,
    "message": "Invalid Request"
  }
}
```

### Method not found (if no upstream and not implemented)

```
{
  "jsonrpc": "2.0",
  "id": 7,
  "error": {
    "code": -32601,
    "message": "Method not found"
  }
}
```

Notes you should codify for tests/docs

- tools/list always returns stable tool name values (use tool_id)
- tools/call governance decisions map to JSON-RPC error (not HTTP status)
- Approval token is accepted only via `arguments._rbitr_approval_token`
- Never leak raw payloads; only safe reasons/rule/risk/policy_version
