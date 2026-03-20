# EPIC 12 — Enterprise Integrations

## Status

| Phase | Status | Date |
|-------|--------|------|
| **1** Chat Integrations (Telegram, WhatsApp) | **DONE** | 2026-03-18 |
| **2** Chat Integrations (Microsoft Teams, Discord) | **SKIPPED** | — |
| **3** Ticketing & ITSM (Jira, ServiceNow, Linear) | **DONE** | 2026-03-20 |
| **4** Observability & SIEM Export | **SKIPPED** | — |
| **5** Identity & SSO (OIDC) | **DONE** | 2026-03-19 |
| **6** Secret Manager Providers | **DONE** | 2026-03-18 |
| **7** Generic Outbound Webhooks | **SKIPPED** | — |

## Summary

Epic 12 extends rbitr's integration surface beyond the current Slack and email channels. The goal is enterprise-grade connectivity across chat, ticketing, observability, identity, and secret management platforms — making rbitr deployable in any corporate environment without custom glue code.

### Current State

rbitr supports three notification channels today:
- **Slack Webhook** — incoming webhook posts
- **Slack Bot** — Slack App Bot API with signing
- **Email** — AWS SES, SendGrid, Mailgun providers

All channels follow the `Notifier` interface (`Send`, `Name`) with per-tenant configuration, deduplication/cooldown, and secret reference resolution (`env://` / `file://`).

---

## Phase 1 — Chat Integrations: Telegram & WhatsApp

### Problem

Many enterprise and regional deployments rely on Telegram or WhatsApp as their primary communication channel. Approval notifications, security alerts, and policy violations must reach operators where they already are.

### Solution

Add two new `Notifier` implementations following the existing Slack pattern.

#### Telegram

- Uses the [Telegram Bot API](https://core.telegram.org/bots/api) (`sendMessage` endpoint)
- Bot token stored as secret ref (`env://` or `file://`)
- Chat ID configured per tenant (group chat or individual)
- Supports Markdown formatting for structured approval/alert messages
- Optional: inline keyboard buttons for approve/deny actions via callback queries

#### WhatsApp

- Uses the [WhatsApp Business Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api) (Meta Graph API)
- Requires a WhatsApp Business Account and phone number ID
- Access token stored as secret ref
- Uses pre-approved message templates for notifications (WhatsApp requirement for business-initiated messages)
- Template parameters populated with approval details, policy decisions, etc.

### Config Model Changes

```sql
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  telegram_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  telegram_secret_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  telegram_chat_id TEXT NOT NULL DEFAULT '';

ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  whatsapp_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  whatsapp_secret_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  whatsapp_phone_number_id TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS
  whatsapp_default_recipient TEXT NOT NULL DEFAULT '';
```

### Implementation

- `internal/notifications/telegram.go` — `TelegramNotifier` implementing `Notifier`
- `internal/notifications/whatsapp.go` — `WhatsAppNotifier` implementing `Notifier`
- Update `NotificationConfig` model with new fields
- Update `Service.buildNotifiers()` to instantiate new channels
- Admin API endpoints for secret refs and test sends
- Migration: `00031_telegram_whatsapp_notifications.sql`

### Admin API Endpoints

```
PUT  /admin/tenants/:tenant_id/notifications/telegram-secret-ref
POST /admin/tenants/:tenant_id/notifications/test/telegram
PUT  /admin/tenants/:tenant_id/notifications/whatsapp-secret-ref
POST /admin/tenants/:tenant_id/notifications/test/whatsapp
```

### Acceptance Criteria

- Tenant can enable Telegram and receive approval/security notifications in a Telegram group
- Tenant can enable WhatsApp and receive templated notifications via WhatsApp Business API
- Both channels respect existing deduplication/cooldown logic
- Secret refs resolved via existing `CompositeResolver`
- Test send endpoints work for both channels

---

## Phase 2 — Chat Integrations: Microsoft Teams & Discord

### Problem

Microsoft Teams is the default chat platform in many enterprises. Discord is common in developer-first and open-source teams. Both are missing from rbitr's notification surface.

### Solution

#### Microsoft Teams

- Uses [Incoming Webhooks](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook) (Adaptive Card JSON payloads)
- Webhook URL stored as secret ref
- Adaptive Card format for structured approval notifications with action buttons
- Optional: Teams Bot Framework integration for interactive approve/deny

#### Discord

- Uses [Discord Webhook API](https://discord.com/developers/docs/resources/webhook)
- Webhook URL stored as secret ref
- Rich embed formatting for structured messages
- Optional: Discord Bot with slash commands for approval management

### Implementation

- `internal/notifications/teams.go` — `TeamsNotifier`
- `internal/notifications/discord.go` — `DiscordNotifier`
- Migration: `00032_teams_discord_notifications.sql`

---

## Phase 3 — Ticketing & ITSM Integration

### Problem

In regulated environments, approval requests and security incidents must create tickets in ITSM systems. Manual ticket creation from rbitr audit events is error-prone and adds operational overhead.

### Solution

Bidirectional integration with ticketing platforms — rbitr creates tickets on specific events and can receive status updates (e.g., ticket resolved = approval granted).

#### Jira

- Create issues on REQUIRE_APPROVAL decisions (project, issue type, priority mapping)
- Update issue status when approval is granted/denied/expired
- Link ADR decision IDs to Jira issue keys for traceability
- Webhook receiver for Jira status transitions → trigger approval actions
- JQL-based query for approval status sync

#### ServiceNow

- Create incidents or change requests on policy decisions
- Map rbitr severity → ServiceNow priority
- CMDB integration for agent/tool asset mapping
- REST Table API for CRUD operations

#### Linear (lightweight alternative)

- Create issues on approval events
- Label-based workflow mapping
- Webhook receiver for status changes

### Config Model

```sql
CREATE TABLE IF NOT EXISTS rbitr.ticketing_config (
  tenant_id    TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id),
  provider     TEXT NOT NULL DEFAULT '',          -- 'jira', 'servicenow', 'linear'
  enabled      BOOLEAN NOT NULL DEFAULT FALSE,
  base_url     TEXT NOT NULL DEFAULT '',
  secret_ref   TEXT NOT NULL DEFAULT '',          -- API token / OAuth secret
  project_key  TEXT NOT NULL DEFAULT '',          -- Jira project, ServiceNow assignment group
  auto_create  BOOLEAN NOT NULL DEFAULT FALSE,    -- auto-create tickets on REQUIRE_APPROVAL
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Implementation

- `internal/ticketing/` — new package with `TicketProvider` interface
- `internal/ticketing/jira.go` — Jira REST API v3 (ADF descriptions, transitions, Basic Auth + Bearer)
- `internal/ticketing/servicenow.go` — ServiceNow REST Table API (incidents, sys_id lookup, state mapping)
- `internal/ticketing/linear.go` — Linear GraphQL API (issue create/update, state transitions)
- `internal/ticketing/service.go` — orchestrator: `OnApprovalCreated`, `OnApprovalDecided`, `OnApprovalExpired`
- `internal/ticketing/priority.go` — risk-to-priority mapping per provider
- `internal/ticketing/webhook_mapping.go` — inbound webhook status → approve/deny/ignore mapping
- `internal/api/admin/ticketing.go` — admin API handlers: config CRUD, secret refs, test send, webhook receiver
- Webhook receiver: parses Jira/ServiceNow/Linear payloads, verifies HMAC-SHA256 signatures, triggers approval actions
- Fire-and-forget integration from public handlers (OnApprovalCreated) and admin handlers (OnApprovalDecided)
- Expiry scheduler integration (OnApprovalExpired)
- Migration: `00032_ticketing_config.sql`

### Admin API Endpoints

```
GET  /admin/tenants/:tenant_id/ticketing              — Get ticketing config
PUT  /admin/tenants/:tenant_id/ticketing              — Update ticketing config
PUT  /admin/tenants/:tenant_id/ticketing/secret-ref   — Set API token secret ref
PUT  /admin/tenants/:tenant_id/ticketing/webhook-secret-ref — Set webhook signing secret ref
POST /admin/tenants/:tenant_id/ticketing/test         — Create test ticket
GET  /admin/tenants/:tenant_id/ticketing/links        — List ticket links
POST /admin/webhooks/ticketing/:provider              — Inbound webhook receiver
```

### Admin Scopes

- `admin:ticketing:read` — view ticketing config and ticket links
- `admin:ticketing:write` — update config, set secret refs
- `admin:ticketing:test` — trigger test ticket creation

---

## Phase 4 — Observability & SIEM Export

### Problem

Security teams need rbitr audit events and policy decisions flowing into their existing observability and SIEM platforms for centralized monitoring, alerting, and forensics.

### Solution

Structured event export to observability platforms. Events include ADR decisions, policy evaluations, approval lifecycle, and security incidents.

#### Supported Targets

| Target | Protocol | Use Case |
|--------|----------|----------|
| **Datadog** | HTTP API (logs/events) | APM-centric teams |
| **Splunk** | HEC (HTTP Event Collector) | Enterprise SIEM |
| **Elastic / OpenSearch** | Bulk API | Self-hosted SIEM |
| **Syslog** | RFC 5424 (TCP/UDP/TLS) | Legacy / on-prem SIEM |
| **OTEL Collector** | OTLP (gRPC/HTTP) | Cloud-native observability |

#### Event Schema

All exports use a common envelope:

```json
{
  "event_type": "POLICY.DECISION",
  "timestamp": "2026-03-18T10:00:00Z",
  "tenant_id": "t1",
  "decision_id": "d-abc123",
  "severity": "HIGH",
  "action_type": "file.write",
  "outcome": "REQUIRE_APPROVAL",
  "metadata": { ... }
}
```

### Implementation

- `internal/export/` — new package with `Exporter` interface
- Async event pipeline: audit events → buffered channel → batch export
- Configurable batch size, flush interval, retry with backoff
- System-level config (not per-tenant) — exports all tenant events
- Migration: `00034_export_config.sql`

---

## Phase 5 — Identity & SSO (OIDC)

### Problem

Admin authentication currently uses HMAC-hashed API keys. Enterprise deployments require SSO via corporate identity providers (Okta, Azure AD, Google Workspace) for admin console access.

### Solution

- OIDC (OpenID Connect) provider integration for admin authentication
- Automatic discovery via `.well-known/openid-configuration`
- HMAC-SHA256 signed admin session tokens post-SSO authentication
- Email domain-based access control (allowed domains list)
- Configurable default scopes for SSO-authenticated admins
- API key auth retained as fallback for programmatic access (dual auth)

### Supported Providers

Any OIDC-compliant identity provider works out of the box:

| Provider | Issuer URL Example |
|----------|-------------------|
| **Google Workspace** | `https://accounts.google.com` |
| **AWS IAM Identity Center** | `https://your-sso-portal.awsapps.com/start` |
| **Okta** | `https://your-org.okta.com` |
| **Azure AD / Entra ID** | `https://login.microsoftonline.com/{tenant-id}/v2.0` |
| **Auth0** | `https://your-domain.auth0.com` |
| **Keycloak** | `https://keycloak.example.com/realms/{realm}` |

### Configuration

SSO configuration is stored in `rbitr.system_settings` (no new migration required). Configuration can be set via environment variables at startup or via the admin API / Settings UI at runtime.

#### Environment Variables

| Key | Value | Description |
|-----|-------|-------------|
| `RBTR_SSO_ENABLED` | `true` | Enable SSO/OIDC authentication |
| `RBTR_SSO_ISSUER` | `https://accounts.google.com` | OIDC issuer URL (used for discovery) |
| `RBTR_SSO_CLIENT_ID` | `your-client-id` | OAuth2 client ID from your IdP |
| `RBTR_SSO_CLIENT_SECRET_REF` | `env://SSO_CLIENT_SECRET` | Secret reference for the OAuth2 client secret |
| `RBTR_SSO_REDIRECT_URI` | `https://rbitr.example.com/admin/auth/sso/callback` | OAuth2 callback URL |
| `RBTR_SSO_ALLOWED_DOMAINS` | `example.com,corp.example.com` | Comma-separated list of allowed email domains |
| `RBTR_SSO_DEFAULT_SCOPES` | `admin:read,admin:write` | Default rbitr scopes for SSO users (default: `admin:read,admin:write`) |

#### System Setting Keys

| System Setting Key | Description |
|--------------------|-------------|
| `sso_enabled` | Toggle SSO on/off |
| `sso_issuer` | OIDC issuer URL |
| `sso_client_id` | OAuth2 client ID |
| `sso_client_secret_ref` | Secret ref for client secret |
| `sso_redirect_uri` | OAuth2 redirect URI |
| `sso_allowed_domains` | Comma-separated allowed email domains |
| `sso_default_scopes` | Comma-separated default admin scopes |

### Admin API Endpoints

```
GET  /admin/auth/sso/config     — Get SSO configuration
PUT  /admin/settings/sso-enabled — Toggle SSO enabled/disabled
PUT  /admin/settings/sso-config  — Update SSO configuration fields
GET  /admin/auth/sso/authorize   — Get IdP authorization URL (starts OIDC flow)
GET  /admin/auth/sso/callback    — Handle OIDC callback, exchange code, issue session
POST /admin/auth/sso/logout      — Revoke SSO admin session
```

### Authentication Flow

1. Admin clicks "Login with SSO" in the UI
2. Frontend calls `GET /admin/auth/sso/authorize` → receives `authorize_url` and `state`
3. Frontend redirects to IdP authorization URL
4. User authenticates with IdP (Google, Okta, etc.)
5. IdP redirects back with authorization code
6. Frontend calls `GET /admin/auth/sso/callback?code=...` → receives session token
7. Frontend stores session token and uses it as `Authorization: Bearer <token>` for subsequent requests
8. Dual auth middleware checks for SSO session first, falls back to API key

### Dual Auth (SSO + API Key)

All admin endpoints support both authentication methods:
- **SSO session**: `Authorization: Bearer <admin-session-token>` (tokens containing `rbas_` prefix in payload)
- **API key**: `Authorization: Bearer <api-key>` or `X-Admin-Key: <api-key>` (existing behavior)

The middleware detects admin session tokens by checking for the `rbas_` prefix, validates the HMAC-SHA256 signature, and checks scopes. If the token is not an admin session, it falls back to API key authentication.

### Implementation

- `internal/auth/oidc.go` — OIDC provider: discovery, authorization URL, code exchange, ID token validation
- `internal/auth/admin_session.go` — Admin session manager: HMAC-SHA256 signed tokens, TTL cache, revocation
- `internal/api/admin/sso.go` — SSO API handlers: config CRUD, authorize, callback, logout
- `internal/api/admin/handlers.go` — Updated `Dependencies` struct, dual auth middleware
- `internal/config/config.go` — SSO environment variable loading
- `internal/store/store.go` — SSO config persistence in system_settings
- Settings UI: SSO configuration card with toggle, text inputs, and save button
- No new migration needed — uses existing `rbitr.system_settings` table

### Acceptance Criteria

- Admin can configure SSO with any OIDC-compliant IdP via UI or API
- Admin can authenticate via SSO and receive a session token
- SSO sessions respect email domain restrictions
- API key auth continues to work alongside SSO (dual auth)
- SSO sessions can be revoked via logout endpoint
- All SSO configuration changes are audit-logged

---

## Phase 6 — Secret Manager Providers

### Problem

The current secret resolver supports `env://` and `file://` references. Enterprise deployments store secrets in cloud-managed vaults, not environment variables or mounted files.

### Solution

Extend `CompositeResolver` with new `SecretProvider` implementations:

| Provider | URI Scheme | Example |
|----------|-----------|---------|
| **AWS Secrets Manager** | `aws-sm://` | `aws-sm://rbitr/slack-token` |
| **GCP Secret Manager** | `gcp-sm://` | `gcp-sm://projects/myproj/secrets/slack-token` |
| **HashiCorp Vault** | `vault://` | `vault://secret/data/rbitr/slack` |
| **Azure Key Vault** | `azure-kv://` | `azure-kv://myvault/slack-token` |

### Configuration

Each provider is opt-in via an environment variable **and** a DB-backed toggle in the admin UI (Settings > Secret providers). Both must be enabled for the provider to be active at runtime — the env var controls startup wiring, the DB toggle controls the system setting.

#### AWS Secrets Manager

| Key | Value | Description |
|-----|-------|-------------|
| `RBTR_SECRET_PROVIDER_AWS` | `true` | Enable the AWS Secrets Manager provider |
| `AWS_ACCESS_KEY_ID` | `AKIA...` | AWS access key (or use IAM role / instance profile) |
| `AWS_SECRET_ACCESS_KEY` | `wJal...` | AWS secret key (or use IAM role / instance profile) |
| `AWS_REGION` | `us-east-1` | AWS region for Secrets Manager API calls |

- **URI scheme**: `aws-sm://<secret-id>`
- **Examples**: `aws-sm://rbitr/slack-token`, `aws-sm://prod/api-keys/stripe`
- **Auth**: Uses AWS SDK v2 default credential chain (env vars, shared credentials file, IAM role, ECS task role, EC2 instance profile)
- **Note**: Only `SecretString` values are supported; binary secrets return an error

#### GCP Secret Manager

| Key | Value | Description |
|-----|-------|-------------|
| `RBTR_SECRET_PROVIDER_GCP` | `true` | Enable the GCP Secret Manager provider |
| `GCP_SECRET_MANAGER_TOKEN` | `ya29.a0...` | OAuth2 access token for the Secret Manager API |

- **URI scheme**: `gcp-sm://<resource-name>`
- **Examples**: `gcp-sm://projects/myproj/secrets/slack-token`, `gcp-sm://projects/myproj/secrets/token/versions/3`
- **Auth**: Set `GCP_SECRET_MANAGER_TOKEN` explicitly, or run on GCE/GKE/Cloud Run where a metadata server token is fetched automatically
- **Note**: If no `/versions/` segment is present in the ref, `/versions/latest` is appended automatically

#### HashiCorp Vault

| Key | Value | Description |
|-----|-------|-------------|
| `RBTR_SECRET_PROVIDER_VAULT` | `true` | Enable the HashiCorp Vault provider |
| `VAULT_ADDR` | `https://vault.example.com:8200` | Vault server address |
| `VAULT_TOKEN` | `hvs.CAESI...` | Vault authentication token |

- **URI scheme**: `vault://<kv-v2-path>` or `vault://<kv-v2-path>#<key>`
- **Examples**: `vault://secret/data/rbitr/slack#token`, `vault://secret/data/app/single`
- **Auth**: Static token via `VAULT_TOKEN` environment variable
- **Note**: Only KV v2 engine is supported (paths must follow `secret/data/...` convention). If the secret has multiple keys, use `#key` to select one; if it has exactly one key, the value is returned directly

#### Azure Key Vault

| Key | Value | Description |
|-----|-------|-------------|
| `RBTR_SECRET_PROVIDER_AZURE` | `true` | Enable the Azure Key Vault provider |
| `AZURE_KEY_VAULT_TOKEN` | `eyJ0eX...` | OAuth2 bearer token for the Key Vault API |

- **URI scheme**: `azure-kv://<vault-name>/<secret-name>`
- **Examples**: `azure-kv://myvault/slack-token`, `azure-kv://prod-vault/api-key`
- **Auth**: Set `AZURE_KEY_VAULT_TOKEN` with a valid OAuth2 token for `https://vault.azure.net` scope
- **Note**: The vault URL is derived as `https://<vault-name>.vault.azure.net`. Uses API version 7.4.

### Admin UI Toggle

All four providers appear as toggle switches in **Settings > Secret providers**. The DB-backed toggle persists across restarts and is stored in `rbitr.system_settings` under these keys:

| System Setting Key | Default |
|--------------------|---------|
| `secret_provider_aws` | `false` |
| `secret_provider_gcp` | `false` |
| `secret_provider_vault` | `false` |
| `secret_provider_azure` | `false` |

### Implementation

- `internal/notifications/secrets_aws.go` — AWS Secrets Manager provider
- `internal/notifications/secrets_gcp.go` — GCP Secret Manager provider
- `internal/notifications/secrets_vault.go` — HashiCorp Vault provider (raw HTTP, no Vault SDK dependency)
- `internal/notifications/secrets_azure.go` — Azure Key Vault provider (raw HTTP, no Azure SDK dependency)
- `cmd/gateway/secret_providers.go` — SDK adapter types and startup wiring
- All providers share the existing 5-minute TTL cache via `CompositeResolver`
- No migration needed — extends existing secret ref resolution

---

## Phase 7 — Generic Outbound Webhooks

### Problem

Not every integration target has a dedicated provider. Teams need a way to push rbitr events to arbitrary HTTP endpoints for custom workflows (internal tools, Zapier, n8n, custom dashboards).

### Solution

- Configurable outbound webhook per tenant
- HTTP POST with JSON payload on configurable event types
- HMAC-SHA256 request signing for webhook verification
- Retry with exponential backoff (3 attempts)
- Delivery log with status tracking

### Config Model

```sql
CREATE TABLE IF NOT EXISTS rbitr.outbound_webhooks (
  webhook_id   TEXT PRIMARY KEY,
  tenant_id    TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
  url          TEXT NOT NULL,
  secret_ref   TEXT NOT NULL DEFAULT '',
  event_types  TEXT[] NOT NULL DEFAULT '{}',
  enabled      BOOLEAN NOT NULL DEFAULT TRUE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Implementation

- `internal/webhooks/` — new package
- `internal/webhooks/dispatcher.go` — async dispatch with retry
- Admin API: CRUD for webhooks, delivery log query, test send
- Migration: `00036_outbound_webhooks.sql`

---

## Implementation Priority

| Phase | Integration | Effort | Enterprise Impact |
|-------|------------|--------|-------------------|
| **1** | Telegram & WhatsApp | Low | Regional/mobile-first teams |
| **2** | Microsoft Teams & Discord | Low | Enterprise chat coverage |
| **3** | Ticketing (Jira, ServiceNow) | High | Regulated environments |
| **4** | Observability & SIEM | Medium | Security operations |
| **5** | Identity & SSO | High | Enterprise admin auth |
| **6** | Secret Manager Providers | Medium | Cloud-native deployments |
| **7** | Generic Outbound Webhooks | Low | Custom integrations |

Completed order: **1 → 6 → 5 → 3**. All planned phases are complete.

Phases 2 (Teams/Discord), 4 (Observability & SIEM), and 7 (Generic Webhooks) are skipped for now.

---

## Definition of Done

- Phase 1: Telegram and WhatsApp notifications delivered with deduplication and cooldown
- Phase 2: Teams and Discord notifications delivered with rich formatting
- Phase 3: Tickets auto-created on approval events; bidirectional status sync
- Phase 4: Audit events exported to at least one SIEM target in structured format
- Phase 5: Admin login via OIDC with IdP group → scope mapping
- Phase 6: Secrets resolved from at least one cloud vault provider
- Phase 7: Outbound webhooks delivered with signing, retry, and delivery log

## Non-Goals for Epic 12

- Inbound message processing (chatbots that receive commands via Telegram/WhatsApp)
- Real-time streaming (WebSocket/SSE) to external systems
- Multi-provider failover (e.g., if Slack fails, fallback to Teams)
- Custom notification template engine (structured messages are code-defined)
