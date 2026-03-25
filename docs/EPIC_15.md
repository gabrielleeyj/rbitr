# EPIC 15 — Agent Activity Governance (Beyond Tool Calls)

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** Activity Event Ingestion | **TODO** | — |
| **2** Full Conversation Audit Trail | **TODO** | — |
| **3** Pre-Execution Policy Evaluation | **TODO** | — |
| **4** Agent Behavior Analytics | **TODO** | — |

## Summary

rbitr currently governs only the tool-call hop — when an agent calls `tools/call` via the MCP endpoint. Everything else the agent does (receiving user messages, LLM reasoning, tool selection, sandbox execution, direct API calls, message responses) is invisible to rbitr. This epic extends governance from tool-call-only to full agent activity, capturing the complete decision chain from user intent to agent action.

Date scoped: 2026-03-25

---

## Problem Statement

```
Telegram User: "Export all customer data and email it to me"
    │
    ▼
Agent (receives message, calls LLM, reasons, picks tools)  ← rbitr sees NONE of this
    │
    ├─ tools/call DATA.EXPORT via rbitr  ← DENIED by policy ✓
    ├─ Agent uses sandbox curl to hit API directly  ← rbitr never knows ✗
    └─ Agent sends response to user  ← rbitr never knows ✗
```

**What rbitr sees today:** Only `tools/call` requests that flow through its MCP endpoint.

**What rbitr misses:**
1. The original user request and intent
2. LLM reasoning and chain-of-thought that led to tool selection
3. Tools the agent considered but didn't call
4. Actions the agent took that bypassed rbitr entirely (sandbox code execution, direct HTTP calls, file operations)
5. The agent's response back to the user
6. Whether the agent attempted to circumvent a prior DENY decision via an alternative path

---

## Governance Levels

| Level | What's Governed | Current State |
|-------|----------------|---------------|
| **L1 — Tool Call** | Individual tool invocations via MCP | Implemented (today) |
| **L2 — Activity Audit** | Full agent activity stream (messages, reasoning, tool calls, responses) | This epic |
| **L3 — Intent Policy** | Policy evaluation on user intent before agent acts | This epic |
| **L4 — Behavioral** | Pattern detection across sessions (data exfiltration attempts, privilege escalation) | This epic |

---

## Phase 1 — Activity Event Ingestion

### Problem

Agents perform many actions beyond tool calls. rbitr has no endpoint to receive or store these activity events.

### Solution

Define an Activity Event schema and ingestion endpoint. Agents (or agent frameworks) emit structured events to rbitr as a sidecar, covering the full lifecycle: message received → reasoning → tool selection → execution → response.

#### Activity Event Types

| Event Type | Description | Example |
|------------|-------------|---------|
| `MESSAGE.RECEIVED` | User message received by agent | "Export all customer data" |
| `MESSAGE.SENT` | Agent response sent to user | "I've processed your refund" |
| `LLM.CALL` | Agent called LLM for reasoning | Model, token count, latency |
| `TOOL.SELECTED` | Agent decided to call a tool | Tool name, reasoning |
| `TOOL.BYPASSED` | Agent performed action outside rbitr | Direct HTTP call, sandbox exec |
| `TOOL.DENIED_RETRY` | Agent retried after a DENY via different path | Circumvention attempt |
| `SESSION.START` | Agent session/conversation started | User ID, channel |
| `SESSION.END` | Agent session/conversation ended | Duration, action count |

#### Ingestion Endpoint

```
POST /v1/activity/:tenant_id
Authorization: Bearer <tenant_key>
X-Agent-Id: openclaw-telegram

{
  "events": [
    {
      "event_type": "MESSAGE.RECEIVED",
      "timestamp": "2026-03-25T08:24:18Z",
      "session_id": "sess_abc123",
      "data": {
        "channel": "telegram",
        "user_id": "347572321",
        "content_hash": "sha256:...",
        "content_preview": "Export all customer data..."
      }
    }
  ]
}
```

### Tasks

- [ ] Design Activity Event schema and storage table
- [ ] Implement `POST /v1/activity/:tenant_id` ingestion endpoint
- [ ] Batch insert with deduplication (idempotency on event_id)
- [ ] Link activity events to existing ADR records via `session_id` and `request_id`
- [ ] Add activity event viewer in admin UI
- [ ] Define agent SDK / integration guide for emitting events
- [ ] Write tests for ingestion, deduplication, and linking

---

## Phase 2 — Full Conversation Audit Trail

### Problem

The audit trail today shows isolated tool-call decisions. There is no way to see the conversation context that led to a tool call, or what happened after a DENY.

### Solution

Stitch activity events into a conversation timeline, providing a complete narrative: what the user asked → what the agent reasoned → what tools were called → what decisions rbitr made → what the agent responded.

### Tasks

- [ ] Implement conversation timeline API: `GET /admin/:tenant_id/sessions/:session_id/timeline`
- [ ] Join activity events with ADR records on session_id
- [ ] Add conversation timeline view in admin UI
- [ ] Support filtering by event type, decision, and time range
- [ ] Add session summary (total actions, decisions, duration)

---

## Phase 3 — Pre-Execution Policy Evaluation

### Problem

Today, policy is evaluated at tool-call time. By then, the agent has already decided what to do. There is no way to evaluate policy on the user's intent before the agent begins acting.

### Solution

Add an optional `intent/evaluate` endpoint that agents can call before acting. The agent sends the user's request and its planned actions; rbitr evaluates against policy and returns guidance (proceed, warn, block).

#### Endpoint

```
POST /v1/intent/:tenant_id/evaluate
Authorization: Bearer <tenant_key>
X-Agent-Id: openclaw-telegram

{
  "session_id": "sess_abc123",
  "user_message": "Export all customer data and email it to me",
  "planned_actions": [
    { "tool": "mock_internal", "action_type": "DATA.EXPORT", "path": "/export_customer_data" },
    { "tool": "email", "action_type": "COMMS.SEND", "recipient": "user@personal.com" }
  ]
}
```

Response:

```json
{
  "evaluation": "BLOCK",
  "reasons": [
    { "action_type": "DATA.EXPORT", "decision": "DENY", "rule_id": "rule_deny_sensitive_v1" },
    { "action_type": "COMMS.SEND", "decision": "ALLOW" }
  ],
  "guidance": "DATA.EXPORT is denied by policy. The agent should inform the user that data export is not permitted."
}
```

### Tasks

- [ ] Implement `POST /v1/intent/:tenant_id/evaluate` endpoint
- [ ] Evaluate each planned action against current policy (batch evaluation)
- [ ] Return per-action decisions with overall guidance
- [ ] Log intent evaluations in audit trail
- [ ] Define integration pattern for agent frameworks (pre-action hook)
- [ ] Write tests for intent evaluation with mixed decisions

---

## Phase 4 — Agent Behavior Analytics

### Problem

Individual tool-call governance catches policy violations one at a time. It cannot detect patterns across sessions: an agent systematically probing for data export paths, escalating privileges across multiple calls, or retrying denied actions via alternative tools.

### Solution

Aggregate activity events and ADR records to detect behavioral patterns and anomalies.

#### Detection Rules

| Pattern | Description | Signal |
|---------|-------------|--------|
| **Deny retry** | Agent retries a denied action via different tool/path | Same `session_id`, same `action_type`, different `tool_id` after DENY |
| **Privilege escalation** | Agent requests increasing permission levels | Sequential `ACCESS.ROLE_CHANGE` calls with escalating roles |
| **Data exfiltration probe** | Agent systematically tries export paths | Multiple `DATA.*` action types in short window |
| **Policy circumvention** | Agent bypasses rbitr for actions that should be governed | `TOOL.BYPASSED` events for action types that have DENY rules |

### Tasks

- [ ] Implement sliding-window event aggregation per session and agent
- [ ] Define behavioral detection rules (configurable per tenant)
- [ ] Add alerting: notify admin on pattern match (webhook, Slack, email)
- [ ] Add behavioral analytics dashboard in admin UI
- [ ] Write tests for pattern detection logic

---

## Dependencies

- **Phase 1** is independent and can start immediately.
- **Phase 2** depends on Phase 1 (needs activity events to build timeline).
- **Phase 3** is independent (uses existing policy engine, no activity events needed).
- **Phase 4** depends on Phase 1 + Phase 2 (needs event stream for pattern detection).

```
Phase 1 (Activity Ingestion)
  ├── Phase 2 (Conversation Audit)
  │     └── Phase 4 (Behavioral Analytics)
  │
Phase 3 (Intent Evaluation) ── independent
```

---

## Integration Requirements

This epic requires cooperation from the agent side. rbitr cannot observe what it isn't told about. Integration approaches by agent framework:

| Framework | Integration Path |
|-----------|-----------------|
| **OpenClaw** | Plugin or middleware that emits activity events to rbitr |
| **Claude Code** | MCP server hooks (PostToolUse) to emit events |
| **LangChain/LangGraph** | Callback handler that posts events to rbitr |
| **Anthropic Agent SDK** | Tool-use hooks or custom middleware |
| **Custom agents** | HTTP POST to `/v1/activity/:tenant_id` at each lifecycle step |

The intent evaluation endpoint (Phase 3) is agent-initiated — the agent must choose to call it before acting. This is opt-in governance, not enforcement. Full enforcement requires the agent framework to route all actions through rbitr, which is a deeper integration than most frameworks support today.

---

## Relationship to Other Epics

- **EPIC 13 (OpenTelemetry Trace Correlation)** — Complements this epic. OTel provides distributed tracing across services; this epic provides semantic activity governance. They can share `trace_id` for cross-correlation.
- **EPIC 14 (HTTP Tool Connector)** — Phase 1 tool CRUD enables the `TOOL.BYPASSED` detection in Phase 4 (rbitr knows which tools exist and can flag calls that went around them).
