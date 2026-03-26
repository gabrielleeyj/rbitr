# Demo Setup: rbitr + OpenClaw + Telegram

## Overview

This demo runs rbitr as an AI governance gateway with OpenClaw as the agent frontend, connected to Telegram for user interaction. The full flow:

```
Telegram User
    │
    ▼
OpenClaw (AI agent, ghcr.io/openclaw/openclaw)
    │
    ▼ MCP (Streamable HTTP)
rbitr Gateway (policy evaluation, audit, metering)
    │
    ▼ HTTP proxy
Mocktool (simulated business tools)
```

All services run locally via Docker Compose — no cloud infrastructure required.

**Free Trial:** rbitr includes a 14-day free trial that starts when you create your first tenant. During the trial, all premium features (approval workflows, integrations, evidence export) are unlocked. After 14 days, these features require a license key.

---

## Architecture

### How Agents Connect to rbitr

rbitr exposes an **MCP (Model Context Protocol) endpoint** that AI agents use to discover and invoke tools. This is the standard integration path — agents should use MCP, not raw HTTP/curl.

**MCP Endpoint:** `POST /v1/mcp/:tenant_id`

**Authentication:**
- `Authorization: Bearer <tenant_key>` — identifies the tenant
- `X-Agent-Id: <agent_name>` — **required**, identifies the calling agent for audit trail

**Protocol:** JSON-RPC 2.0 over HTTP (MCP Streamable HTTP transport)

### MCP Request Flow

1. **Tool Discovery** — Agent calls `tools/list` to discover available tools
2. **Tool Invocation** — Agent calls `tools/call` with tool name and arguments
3. **Policy Evaluation** — rbitr evaluates the call against the tenant's Rego policy
4. **Decision** — ALLOW (proxied to upstream), REQUIRE_APPROVAL (queued), or DENY (rejected)
5. **Audit** — Every call is logged with agent ID, decision, policy rule, and timestamp

### Example MCP Calls

**List tools:**
```json
POST /v1/mcp/t_ad99d867
Authorization: Bearer <tenant_key>
X-Agent-Id: my-agent
Content-Type: application/json

{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```

**Call a tool:**
```json
POST /v1/mcp/t_ad99d867
Authorization: Bearer <tenant_key>
X-Agent-Id: my-agent
Content-Type: application/json

{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mock_internal","arguments":{"action_type":"PAYMENT.REFUND","path":"/refund","amount":"50"}}}
```

### Mocktool Endpoints

The mock tool simulates three sensitive business operations for testing policy enforcement:

| Endpoint | Action Type | Default Policy Decision |
|----------|-------------|------------------------|
| `/refund` | `PAYMENT.REFUND` | REQUIRE_APPROVAL |
| `/export_customer_data` | `DATA.EXPORT` | DENY |
| `/change_role` | `ACCESS.ROLE_CHANGE` | REQUIRE_APPROVAL |

---

## Prerequisites

1. **Docker and Docker Compose** installed
2. **Telegram bot token** — create via `@BotFather` on Telegram
3. **Anthropic API key** — for the LLM backing OpenClaw

---

## Quick Start

### 1. Configure environment

```bash
cp .env.demo.example .env.demo
# Edit .env.demo and set:
#   TELEGRAM_BOT_TOKEN=<from @BotFather>
#   ANTHROPIC_API_KEY=<your key>
```

### 2. Start all services

```bash
docker compose -f docker-compose.demo.yml --env-file .env.demo up --build
```

This starts: Postgres, migrations, rbitr gateway, UI, OpenClaw, and mocktool.

### 3. Initialize rbitr tenant

Open the rbitr UI at `http://localhost:5173` and complete setup, or via API:

```bash
curl -s -X POST http://localhost:8080/setup/initialize \
  -H "Content-Type: application/json" \
  -d '{"tenant_name": "demo"}' | python3 -m json.tool
```

Save the returned `admin_key`, `tenant_key`, and `tenant_id`.

### 4. Configure OpenClaw MCP server

Add the tenant credentials to `.env.demo`:

```bash
# Add these lines to .env.demo:
RBITR_TENANT_ID=<tenant_id from step 3>
RBITR_TENANT_KEY=<tenant_key from step 3>
```

Then restart OpenClaw to apply the MCP configuration:

```bash
docker compose -f docker-compose.demo.yml restart openclaw
```

### 5. Pair Telegram

Message your bot on Telegram. OpenClaw will return a pairing code. Approve it:

```bash
docker exec rbitr-openclaw-1 openclaw pairing approve <CODE>
```

### 6. Test MCP integration

Now OpenClaw has the rbitr MCP server pre-configured. Ask the agent via Telegram:

> List available tools from the rbitr server, then call mock_internal to process a $5 refund.

The agent should be able to list tools and make governed calls through rbitr.

### 7. Verify audit trail

Check the gateway logs:

```bash
docker compose -f docker-compose.demo.yml logs gateway --tail 20
```

Or view the audit trail in the rbitr UI at `http://localhost:5173`.

---

## Service Ports

| Service | Port | URL |
|---------|------|-----|
| rbitr Gateway | 8080 | `http://localhost:8080` |
| rbitr UI | 5173 | `http://localhost:5173` |
| OpenClaw Gateway | 18789 | `http://localhost:18789` |
| OpenClaw Bridge | 18790 | `http://localhost:18790` |
| Mocktool | 8090 | `http://localhost:8090` |
| Postgres | 2345 | `localhost:2345` |

---

## Integration Patterns for Other Agents

Any MCP-compatible agent can integrate with rbitr using the same pattern:

### NanoClaw

Set `ANTHROPIC_BASE_URL` to point at rbitr's MCP endpoint, or use the `/add-telegram` skill and configure MCP server in the agent's tool config.

### Claude Code / Claude Desktop

Add rbitr as an MCP server in the agent's MCP configuration:

```json
{
  "mcpServers": {
    "rbitr": {
      "url": "http://localhost:8080/v1/mcp/<tenant_id>",
      "headers": {
        "Authorization": "Bearer <tenant_key>",
        "X-Agent-Id": "claude-code"
      }
    }
  }
}
```

### Custom Agents (Anthropic Agent SDK, LangChain, etc.)

Any agent that supports MCP Streamable HTTP transport can connect. The minimum integration:

1. Call `tools/list` at startup to discover available tools
2. Call `tools/call` when the LLM selects a tool
3. Include `Authorization` and `X-Agent-Id` headers on every request

### Direct HTTP (non-MCP agents)

Agents that don't support MCP can use the REST tool call endpoint:

```bash
POST /v1/tools/<tool_id>/call
Authorization: Bearer <tenant_key>
X-Agent-Id: my-agent
Content-Type: application/json

{"tool_id":"refund","action_type":"PAYMENT.REFUND","arguments":{"amount":"50"}}
```

---

## Troubleshooting

### OpenClaw exits with "Config invalid"

OpenClaw has a strict config schema. Do not add unrecognized keys to `openclaw.json`. MCP configuration should be handled via the agent's chat context or a supported plugin.

### OpenClaw exits with "EACCES: permission denied"

The OpenClaw image runs as `node` (uid 1000). Ensure bind-mounted config files are copied to a writable directory (not mounted read-only at the target path). The demo compose copies configs to `/home/node/.openclaw/` via entrypoint.

### MCP auth returns "authentication failed"

- Verify `X-Agent-Id` header is present (required, non-empty)
- Verify `Authorization: Bearer <tenant_key>` matches the key in the database
- Verify the `tenant_id` in the URL matches the tenant associated with the key

### No audit trail in gateway logs

- Verify the agent is calling rbitr's MCP endpoint, not the tool directly
- Check `docker compose logs gateway --tail 20` for requests to `/v1/mcp/`
- If the agent uses curl directly to mocktool, it bypasses rbitr entirely

### Migration fails with "dial error"

If using `--env-file`, ensure `$$DATABASE_URL` (double dollar) is used in the migrate entrypoint to prevent Docker Compose from substituting the variable.
