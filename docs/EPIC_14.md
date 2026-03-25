# EPIC 14 — HTTP Tool Connector: CRUD, Discovery & Credential Management

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** Tool CRUD API & UI | **TODO** | — |
| **2** Dev/Mock Tool Isolation | **TODO** | — |
| **3** OpenAPI Auto-Discovery | **TODO** | — |
| **4** Credential Lifecycle (OAuth2, Vault) | **TODO** | — |
| **5** Agent Tool Discovery via MCP | **TODO** | — |

## Summary

Epic 14 adds full lifecycle management for HTTP tool connectors in rbitr. Today, tools can only be created during tenant initialization (hard-coded dev seed) and updated via admin API — there is no way to create, delete, or discover tools dynamically. This epic introduces tool CRUD, dev/prod isolation, OpenAPI-based auto-discovery, and credential management for short-lived tokens — enabling a seamless experience for admins configuring tools and agents consuming them.

Date scoped: 2026-03-25

---

## Problem Statement

1. **No tool creation or deletion API.** Tools are only inserted during `POST /setup/initialize` via a dev seed (`RBTR_DEV_AUTO_TOOLS=true`). Production users have no way to register their own HTTP tools.
2. **Dev tools leak into production.** `mock_internal` and `jira` dev seeds are inserted unconditionally when the dev flag is set. There is no environment-based isolation — a production gateway with the flag accidentally enabled exposes mock endpoints.
3. **Agents cannot discover HTTP tool capabilities.** `tools/list` returns tool names and schemas, but for HTTP-backed tools, agents have no way to know which endpoints, methods, or arguments are available without being told out-of-band (e.g., pasted into chat).
4. **Short-lived credentials break.** HTTP connectors store `auth_value` (API key or bearer token) as a static string. OAuth2 tokens, rotating API keys, and other short-lived credentials expire, requiring manual admin API calls to update. There is no automatic refresh.
5. **No soft-delete for audit integrity.** Deleting a tool would orphan ADR (audit) records that reference the `tool_id`. There is no soft-delete or archival mechanism.

---

## Phase 1 — Tool CRUD API & UI

### Problem

Admins can only update existing tools — they cannot create new ones or remove unused ones. The only way to add a tool is to modify the Go source code or insert directly into the database.

### Solution

Add full CRUD endpoints for tools under the existing admin API, with soft-delete to preserve audit trail integrity.

#### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/admin/:tenant_id/tools` | Create a new tool |
| `GET` | `/admin/:tenant_id/tools` | List all tools (existing) |
| `GET` | `/admin/:tenant_id/tools/:tool_id` | Get tool details |
| `PUT` | `/admin/:tenant_id/tools/:tool_id` | Update tool config (existing) |
| `PUT` | `/admin/:tenant_id/tools/:tool_id/metadata` | Update tool metadata (existing) |
| `DELETE` | `/admin/:tenant_id/tools/:tool_id` | Soft-delete (archive) a tool |
| `POST` | `/admin/:tenant_id/tools/:tool_id/restore` | Restore a soft-deleted tool |

#### Create Tool Request

```json
{
  "tool_id": "payment-api",
  "base_url": "https://api.stripe.com/v1",
  "auth_type": "bearer",
  "auth_value": "sk_live_...",
  "description": "Stripe payment processing API",
  "transport": "http",
  "input_schema_json": {
    "type": "object",
    "properties": {
      "path": { "type": "string", "description": "API endpoint path" },
      "method": { "type": "string", "enum": ["GET", "POST", "PUT", "DELETE"] },
      "amount": { "type": "integer", "description": "Amount in cents" }
    },
    "required": ["path"]
  }
}
```

#### Soft-Delete Design

- Add `archived_at TIMESTAMPTZ NULL` column to `rbitr.tools` table.
- `DELETE` sets `archived_at = now()` instead of removing the row.
- `tools/list` (MCP) and `GET /admin/:tenant_id/tools` exclude archived tools by default.
- ADR foreign key references remain valid since the row persists.
- `POST .../restore` clears `archived_at` to re-enable the tool.
- Admin list endpoint accepts `?include_archived=true` query param.

#### Validation Rules

| Field | Rule |
|-------|------|
| `tool_id` | Required, alphanumeric + underscores/hyphens, 3-64 chars, unique per tenant |
| `base_url` | Required for HTTP transport, must be valid `http://` or `https://` URL |
| `auth_type` | One of: `none`, `bearer`, `api_key`, `oauth2_client_credentials` |
| `transport` | One of: `http`, `mcp` |
| `input_schema_json` | Optional, must be valid JSON Schema if provided |

#### SSRF Protection

The existing `validateOutboundURL` in `connector/url.go` only checks for valid scheme and host. Extend it to block:

- Private/reserved IP ranges: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `127.0.0.0/8`
- Cloud metadata endpoints: `169.254.169.254`, `fd00::/8`
- Link-local and loopback addresses
- Configurable allowlist via `RBTR_OUTBOUND_ALLOW_PRIVATE=true` for local dev (off by default)

### Tasks

- [ ] Add `archived_at` column to `rbitr.tools` table (migration)
- [ ] Add `InsertTool` store method with tool_id uniqueness check
- [ ] Add `ArchiveTool` store method (soft-delete)
- [ ] Add `RestoreTool` store method
- [ ] Update `ListTools` to exclude archived by default, add `includeArchived` param
- [ ] Implement `POST /admin/:tenant_id/tools` handler with validation
- [ ] Implement `GET /admin/:tenant_id/tools/:tool_id` handler
- [ ] Implement `DELETE /admin/:tenant_id/tools/:tool_id` handler (soft-delete)
- [ ] Implement `POST /admin/:tenant_id/tools/:tool_id/restore` handler
- [ ] Extend `validateOutboundURL` with private IP / SSRF blocking
- [ ] Add `RBTR_OUTBOUND_ALLOW_PRIVATE` env var for local dev
- [ ] Emit audit events for tool create, archive, and restore
- [ ] Add UI page for tool management (list, create, edit, archive, restore)
- [ ] Write unit tests for all new store methods and handlers
- [ ] Write integration tests for tool lifecycle (create → update → archive → restore)

---

## Phase 2 — Dev/Mock Tool Isolation

### Problem

`RBTR_DEV_AUTO_TOOLS=true` seeds `mock_internal` and `jira` tools into the database during tenant initialization. These tools exist alongside real tools with no distinction. A production deployment that accidentally sets this flag — or one that was initialized in dev mode and later promoted — will have mock tools visible to agents.

### Solution

Introduce a `source` column on tools to distinguish how they were created, and prevent dev-sourced tools from being visible in production mode.

#### Tool Source Types

| Source | Description | Visible in prod? |
|--------|-------------|-------------------|
| `admin` | Created via admin API or UI | Yes |
| `dev_seed` | Auto-created by `RBTR_DEV_AUTO_TOOLS` | No (unless `RBTR_DEV_MODE=true`) |
| `openapi_import` | Imported from OpenAPI spec (Phase 3) | Yes |

#### Behavior

- Dev-seeded tools are tagged `source = 'dev_seed'` at creation time.
- `tools/list` (MCP) filters out `dev_seed` tools when `RBTR_DEV_AUTO_TOOLS` is not set.
- Admin UI shows dev tools with a visual badge ("Dev Only") and a warning.
- `GET /admin/:tenant_id/tools` includes a `source` field in the response.
- Migration backfills `source = 'dev_seed'` for tools matching known dev tool IDs (`mock_internal`, `jira`).

### Tasks

- [ ] Add `source TEXT NOT NULL DEFAULT 'admin'` column to `rbitr.tools` (migration)
- [ ] Backfill `source = 'dev_seed'` for existing `mock_internal` and `jira` tools
- [ ] Update dev seed in `service.go` to set `source = 'dev_seed'`
- [ ] Update `ListTools` store method to filter by source based on runtime mode
- [ ] Update `tools/list` MCP handler to respect source filtering
- [ ] Add "Dev Only" badge in UI for `dev_seed` tools
- [ ] Write tests for source-based filtering

---

## Phase 3 — OpenAPI Auto-Discovery

### Problem

When an admin registers an HTTP tool, agents only see the tool name and whatever `input_schema_json` was manually provided. For real APIs with dozens of endpoints (Stripe, Jira, Slack), manually writing input schemas is impractical. Agents need to discover available endpoints, methods, parameters, and expected responses.

### Solution

Allow admins to import tools from an OpenAPI (Swagger) specification. rbitr parses the spec and auto-generates MCP-compatible tool definitions — one tool per operation or one tool per spec (configurable).

#### Import Modes

| Mode | Description | Use case |
|------|-------------|----------|
| **Single tool** | One MCP tool wrapping the entire API; `path` and `method` are arguments | Simple APIs, fewer tools in agent context |
| **Multi tool** | One MCP tool per OpenAPI operation (e.g., `stripe_create_refund`) | Rich APIs where each operation is distinct |

#### Import Flow

1. Admin provides OpenAPI spec URL or uploads JSON/YAML file via UI or API.
2. rbitr parses the spec, extracts operations, and generates tool definitions.
3. Admin reviews generated tools, selects which to enable, and confirms.
4. Tools are inserted with `source = 'openapi_import'` and `openapi_spec_url` reference.
5. Agent calls `tools/list` and sees the imported tools with full input schemas.

#### API

```
POST /admin/:tenant_id/tools/import/openapi
Content-Type: application/json

{
  "spec_url": "https://api.example.com/openapi.json",
  "mode": "single",
  "base_url_override": "https://api.example.com",
  "auth_type": "bearer",
  "auth_value": "...",
  "prefix": "example"
}
```

Response: preview of tools to be created. Admin confirms with a follow-up `POST .../import/openapi/confirm`.

#### Schema Generation

For each OpenAPI operation, generate an MCP `inputSchema`:

- Path parameters → required properties
- Query parameters → optional properties
- Request body → nested `body` property
- `path` and `method` are auto-populated (not exposed to agent in multi-tool mode)

### Tasks

- [ ] Add OpenAPI spec parser (support OpenAPI 3.0 and 3.1, JSON and YAML)
- [ ] Implement single-tool and multi-tool generation modes
- [ ] Add `openapi_spec_url` and `openapi_operation_id` columns to `rbitr.tools`
- [ ] Implement `POST /admin/:tenant_id/tools/import/openapi` (preview endpoint)
- [ ] Implement `POST /admin/:tenant_id/tools/import/openapi/confirm` (commit endpoint)
- [ ] Add OpenAPI import page in UI with tool preview and selection
- [ ] Handle spec refresh: detect drift between stored tools and updated spec
- [ ] Write tests for OpenAPI parsing and schema generation
- [ ] Write integration test for full import flow

---

## Phase 4 — Credential Lifecycle (OAuth2, Vault)

### Problem

HTTP tool connectors store credentials as static strings (`auth_value`). This works for long-lived API keys but fails for:

- **OAuth2 access tokens** — expire in minutes to hours, require `client_credentials` grant to refresh.
- **Rotating API keys** — some services rotate keys on a schedule.
- **Vault-managed secrets** — enterprises store credentials in HashiCorp Vault, AWS Secrets Manager, or similar; the gateway should fetch at runtime rather than storing a copy.

Admins currently must call `PUT /admin/:tenant_id/tools/:tool_id` every time a token expires — impractical for high-frequency rotation.

### Solution

Introduce a credential provider abstraction. Instead of storing the raw `auth_value`, the tool references a credential provider that resolves the current token at request time.

#### Credential Provider Types

| Provider | Config | Refresh Strategy |
|----------|--------|------------------|
| `static` | `auth_value` stored in DB (current behavior) | None — admin updates manually |
| `oauth2_client_credentials` | `token_url`, `client_id`, `client_secret`, `scope` | Auto-refresh before expiry; cache token in memory with TTL |
| `vault` | `vault_addr`, `vault_path`, `vault_role` | Fetch from Vault at request time; cache with configurable TTL |
| `env` | `env_var` name | Read from environment variable at startup; no refresh |

#### Architecture

```
Agent → MCP tools/call → rbitr
                            │
                            ├─ Resolve credential (provider.Get(ctx, toolID))
                            │     ├─ static: return stored value
                            │     ├─ oauth2: check cache → refresh if expired → return token
                            │     ├─ vault: check cache → fetch from vault → return secret
                            │     └─ env: return os.Getenv(varName)
                            │
                            ├─ Inject into request headers
                            └─ Forward to upstream
```

#### OAuth2 Client Credentials Flow

```json
{
  "auth_type": "oauth2_client_credentials",
  "credential_config": {
    "token_url": "https://auth.example.com/oauth/token",
    "client_id": "rbitr-gateway",
    "client_secret": "...",
    "scope": "api:read api:write",
    "token_cache_ttl_seconds": 300
  }
}
```

The gateway performs the `client_credentials` grant, caches the access token, and injects it as `Authorization: Bearer <token>` on every upstream request. Token refresh happens automatically before expiry — agents never see auth errors.

#### Security Considerations

- `client_secret` and `vault_role` are stored encrypted at rest (AES-256-GCM, key from `RBTR_CREDENTIAL_KEY` env var).
- Admin API never returns `client_secret` or `auth_value` in responses — only `"configured": true/false`.
- OAuth2 token cache is in-memory only; never persisted to disk or database.
- Vault provider uses AppRole or Kubernetes auth — no long-lived Vault tokens.
- Credential resolution failures are logged and metered; agents receive a generic "upstream auth failed" error (no credential leakage).
- Follow Kong's pattern: extract claims from validated tokens and forward as headers — never pass raw OAuth2 access tokens through to upstream services (prevents confused deputy attacks).

### Tasks

- [ ] Design `CredentialProvider` interface: `Get(ctx, toolID) → (token string, err error)`
- [ ] Implement `StaticProvider` (current behavior, backward-compatible)
- [ ] Implement `OAuth2ClientCredentialsProvider` with token caching and auto-refresh
- [ ] Implement `VaultProvider` with AppRole auth and configurable TTL cache
- [ ] Implement `EnvProvider` for environment variable resolution
- [ ] Add `credential_config JSONB` column to `rbitr.tools` (migration)
- [ ] Encrypt sensitive fields in `credential_config` at rest
- [ ] Update `applyToolAuth` to resolve credentials via provider instead of reading `auth_value`
- [ ] Add `RBTR_CREDENTIAL_KEY` env var for encryption key
- [ ] Update admin API: accept `credential_config` on create/update, never return secrets
- [ ] Add credential provider configuration in UI (form per provider type)
- [ ] Add credential health check endpoint: `GET /admin/:tenant_id/tools/:tool_id/credential/status`
- [ ] Write unit tests for each provider (token refresh, cache expiry, vault errors)
- [ ] Write integration test for OAuth2 flow with mock token endpoint

---

## Phase 5 — Agent Tool Discovery via MCP

### Problem

When an agent calls `tools/list`, it receives tool names, descriptions, and input schemas. For HTTP-backed tools, the description and schema quality depend entirely on what the admin provided (or what OpenAPI import generated). Agents have no standardized way to:

- Understand which `path` values are valid for a given tool.
- Know which HTTP methods are supported per path.
- Discover required vs. optional parameters per endpoint.
- Get example requests and responses.

### Solution

Enhance the `tools/list` response for HTTP-backed tools to include richer discovery metadata, derived from OpenAPI specs (Phase 3) or admin-provided schemas.

#### Enhanced Tool Schema

For tools imported from OpenAPI, the `inputSchema` already contains full parameter details. For manually created tools, enhance the schema with:

- `x-rbitr-endpoints`: list of available paths with methods and descriptions.
- `x-rbitr-examples`: example argument sets for common operations.

```json
{
  "name": "payment-api",
  "description": "Stripe payment processing API",
  "inputSchema": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "enum": ["/v1/refunds", "/v1/charges", "/v1/customers"],
        "description": "API endpoint path"
      },
      "method": {
        "type": "string",
        "enum": ["GET", "POST"],
        "description": "HTTP method"
      },
      "amount": { "type": "integer" },
      "currency": { "type": "string" }
    },
    "required": ["path"]
  }
}
```

#### Resource Discovery (MCP resources/list)

For richer discovery, expose tool documentation as MCP resources:

```json
{
  "method": "resources/list",
  "result": {
    "resources": [
      {
        "uri": "rbitr://tools/payment-api/openapi",
        "name": "payment-api OpenAPI spec",
        "mimeType": "application/json"
      }
    ]
  }
}
```

Agents can read the full OpenAPI spec via `resources/read` for deep understanding of the API.

### Tasks

- [ ] Enhance `tools/list` response with richer inputSchema for HTTP tools
- [ ] Add `x-rbitr-endpoints` extension for manually created HTTP tools
- [ ] Implement `resources/list` MCP method to expose tool documentation
- [ ] Implement `resources/read` MCP method to serve OpenAPI specs
- [ ] Add admin UI for managing tool endpoint metadata
- [ ] Write tests for enhanced discovery responses

---

## Architecture Decisions

### Why soft-delete instead of hard-delete?

ADR records reference `tool_id` for audit trail integrity. Hard-deleting a tool would either require cascading deletes (losing audit history) or leave orphaned references. Soft-delete preserves the tool row so historical queries remain valid, while hiding it from active tool lists.

### Why credential providers instead of a simpler token refresh endpoint?

A dedicated "refresh" admin endpoint would require an external cron job or the admin to poll — error-prone and inconsistent. The provider pattern pushes refresh logic into the gateway itself, ensuring credentials are always valid at the moment of use without external coordination.

### Why OpenAPI import instead of manual schema authoring?

Real APIs have dozens to hundreds of endpoints. Manual `input_schema_json` authoring is tedious and error-prone. OpenAPI specs are the industry standard for API documentation, and most APIs publish one. Importing from spec reduces admin effort from hours to seconds and ensures schemas stay accurate.

### Why single-tool and multi-tool import modes?

Different use cases warrant different granularities. A simple internal API might work best as a single tool with `path` as an argument — fewer tools means less context for the LLM. A rich API like Stripe benefits from per-operation tools where each tool name is semantically meaningful (`stripe_create_refund` vs. generic `stripe` with a path argument).

---

## Dependencies

- **Phase 1** is independent and can start immediately.
- **Phase 2** depends on Phase 1 (needs `source` column and tool CRUD).
- **Phase 3** depends on Phase 1 (needs tool creation API for imported tools).
- **Phase 4** depends on Phase 1 (needs updated tool schema for credential config).
- **Phase 5** depends on Phase 3 (OpenAPI-derived schemas for rich discovery).

```
Phase 1 (CRUD + Soft-Delete)
  ├── Phase 2 (Dev Isolation)
  ├── Phase 3 (OpenAPI Import)
  │     └── Phase 5 (Agent Discovery)
  └── Phase 4 (Credential Lifecycle)
```

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| SSRF via admin-configured `base_url` | Attacker with admin key can probe internal network | Private IP blocking in `validateOutboundURL`, configurable allowlist for dev |
| Credential leakage in logs/responses | Tokens exposed in error messages or audit trail | Never log or return `auth_value`/`client_secret`; encrypt at rest |
| OAuth2 token endpoint unavailable | All tool calls fail until token endpoint recovers | Retry with exponential backoff; serve cached token until true expiry; alert admin |
| OpenAPI spec drift | Imported tools diverge from actual API | Spec refresh on schedule or on-demand; diff detection with admin notification |
| Tool ID collisions on import | Multi-tool import generates conflicting IDs | Prefix-based namespacing (`{prefix}_{operation_id}`); uniqueness check before commit |
| Large OpenAPI specs overwhelm agent context | LLM cannot process hundreds of tools | Pagination in `tools/list`; admin selects subset during import; tool grouping |
| Unauthenticated `tools/list` exposes attack surface | Tool schemas act as self-documenting exploit guides (21k+ exposed MCP servers found in the wild with zero auth) | Already mitigated: rbitr requires `Authorization` + `X-Agent-Id` on all MCP endpoints; no anonymous discovery |
