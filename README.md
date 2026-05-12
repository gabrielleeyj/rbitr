# rbitr - the agent governance control plane.

![Unit Tests](https://github.com/gabrielleeyj/rbitr/actions/workflows/go.yml/badge.svg)

## Introduction

Governance semantics for AI agents: canonicalization + hashing + action classification.

- OPA/Rego policies stored in DB (policy-as-data)
- ADR persistence (governance artifact, not access logs)
- Approval-request persistence (the "human-in-loop" hook)
- Evidence export with DTO whitelist + contract validation + redaction tests
- Risk overrides
- Metrics for decisioning and latency

Use case: "Enterprise customer asks: prove your AI agent can't refund/export/change permissions without controls."

You respond with: action policies + approval artifacts + tenant evidence pack + policy snapshot + simulation result.

Governance domains:

- Payments/refunds
- Data export / privacy
- Access/permissions
- Support ops (ticketing + CRM updates)

## Getting Started (Dev)

### Option A: Docker Compose (recommended)

Brings up Postgres, runs migrations, and starts the gateway, UI, and mocktool in one command.

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

- Gateway: http://localhost:8080
- UI: http://localhost:5173
- Postgres: localhost:2345
- Mocktool: http://localhost:8090

Once the gateway is healthy, bootstrap the first tenant + admin keys (run from a second terminal):

```bash
./scripts/dev/bootstrap.sh
```

This creates tenant `t_demo` with admin key `admin_demo_key_2026!` and tenant key `tenant_demo_key_2026!`. Override via `TENANT_ID`, `ADMIN_KEY`, `TENANT_KEY` env vars.

### Option B: Local Binaries

Prerequisites:

- Go 1.25+
- A running Postgres on `localhost:2345` (the easiest path is `docker compose up -d db` from this repo, which exposes Postgres at port 2345)
- `goose` migration tool (`go install github.com/pressly/goose/v3/cmd/goose@latest`)

1. Run migrations:

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:2345/rbitr?sslmode=disable
goose -dir migrations postgres "$DATABASE_URL" up
```

2. Start mock tool and gateway in separate terminals:

```bash
go run ./cmd/mocktool   # terminal 1, port 8090
go run ./cmd/gateway    # terminal 2, port 8080
```

3. Bootstrap the first tenant (terminal 3):

```bash
./scripts/dev/bootstrap.sh
```

4. Run tests: `go test ./...`

### Option C: OpenClaw + Telegram demo

A separate compose file wires rbitr together with the OpenClaw agent host and a Telegram bot for an end-to-end agent governance demo. See `docs/demo-setup.md` for the full walkthrough.

```bash
cp .env.demo.example .env.demo   # fill in TELEGRAM_BOT_TOKEN, ANTHROPIC_API_KEY
docker compose -f docker-compose.demo.yml --env-file .env.demo up --build
```

### Database Runtime Defaults

- If `DATABASE_URL` is not set, the gateway defaults to:
  - `postgres://postgres@localhost:2345/rbitr?sslmode=require`
- Connection pool defaults (overridable via env):
  - `DB_MAX_OPEN_CONNS=30`
  - `DB_MAX_IDLE_CONNS=10`
  - `DB_CONN_MAX_LIFETIME_SECONDS=1800`
  - `DB_CONN_MAX_IDLE_TIME_SECONDS=300`

## First-run Setup

On a fresh deployment, the UI checks bootstrap state and routes to `/setup` until setup is complete.

Setup API endpoints:

- `GET /setup/status`
- `POST /setup/initialize`

Production hardening flags:

- `RBTR_SETUP_TOKEN_REQUIRED=true` enables setup token gate and idempotency enforcement.
- `RBTR_SETUP_TOKEN` or `RBTR_SETUP_TOKEN_FILE` provides the bootstrap setup token.
- `RBTR_SETUP_ALLOWED_CIDRS` (comma-separated CIDRs) optionally restricts initialize callers by source network.
- In token-required mode, `POST /setup/initialize` must include:
  - `Authorization: Bearer <setup_token>`
  - `Idempotency-Key: <client_generated_key>`

Bootstrap sequence:

1. Validate environment readiness (DB connectivity + schema presence)
2. Create initial tenant profile
3. Create admin key and tenant key (auto-generated or user-provided)
4. Seed default policy and activate it for the tenant
5. Mark bootstrap complete

Note: the setup flow validates that migrations are present but does not run migrations itself; run migrations before using `/setup/initialize`.

Dev note: when `RBTR_DEV_AUTO_TOOLS=true`, setup auto-seeds per-tenant dev tool wiring:

- `mock_internal` -> `RBTR_DEV_MOCK_INTERNAL_URL` (default `http://localhost:8090`)
- `jira` -> `RBTR_DEV_JIRA_URL` (default `http://localhost:8081`)

## Production Deployment

Two deployment paths are provided for AWS:

### Helm Chart (EKS)

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm dependency build deploy/helm/rbitr

# Quick-start with bundled PostgreSQL (eval only)
helm install rbitr deploy/helm/rbitr --set postgresql.enabled=true

# Production with external RDS
helm install rbitr deploy/helm/rbitr \
  -f deploy/helm/rbitr/values-production.yaml \
  --set externalDatabase.host=rbitr.abc123.us-east-1.rds.amazonaws.com \
  --set externalDatabase.password=<db-password>
```

Chart features:

- Gateway + UI deployments with health/readiness probes
- ALB ingress with path-based routing
- Migration job (pre-install/pre-upgrade Helm hook with goose)
- Optional bundled PostgreSQL (Bitnami subchart, disabled by default)
- Auto-generated secrets (HMAC keys, setup token) persisted across upgrades
- HPA and IRSA-ready ServiceAccount
- See `deploy/helm/rbitr/values.yaml` for all configuration options

### CloudFormation (ECS Fargate)

Deploy via the AWS Console or CLI using `deploy/cloudformation/rbitr-ecs.yaml`:

```bash
aws cloudformation create-stack \
  --stack-name rbitr \
  --template-body file://deploy/cloudformation/rbitr-ecs.yaml \
  --parameters \
    ParameterKey=CertificateArn,ParameterValue=arn:aws:acm:us-east-1:123456789:certificate/abc \
    ParameterKey=GatewayImageUri,ParameterValue=ghcr.io/gabrielleeyj/rbitr/gateway:v0.1.0 \
    ParameterKey=UIImageUri,ParameterValue=ghcr.io/gabrielleeyj/rbitr/ui:v0.1.0 \
  --capabilities CAPABILITY_IAM
```

Creates: VPC, ECS Fargate cluster, RDS PostgreSQL 16, ALB with HTTPS, Secrets Manager credentials, CloudWatch logs.

### Container Images

Published to GHCR on each release:

- `ghcr.io/gabrielleeyj/rbitr/gateway:<tag>`
- `ghcr.io/gabrielleeyj/rbitr/ui:<tag>`
- `ghcr.io/gabrielleeyj/rbitr/mocktool:<tag>`

All images are multi-arch (linux/amd64, linux/arm64) with SBOM and provenance attestation.

### TLS and Ingress

See `docs/production-ingress.md` for reference configurations covering Nginx TLS, Kubernetes ingress (nginx-ingress, Traefik), AWS ALB, GCP HTTPS LB, cert-manager, and security headers.

## Release Pipeline

Defined in `.github/workflows/release.yml`.

Triggers:

- push tag `v*` (for example `v1.2.3`)
- manual `workflow_dispatch` with an existing release tag

Gates:

- `golangci-lint`
- `go test ./...`
- setup smoke gate (`scripts/test_setup_smoke.sh`)
- marketplace onboarding harness (`scripts/verify_marketplace_onboarding.sh`)

Outputs:

- platform binary archives + `SHA256SUMS`
- onboarding verification report artifact
- GHCR multi-arch images (gateway, mocktool, ui) with SBOM + provenance
- published GitHub Release with attached artifacts

## Marketplace Onboarding Verification

Use the onboarding-aware harness to validate:

- fresh install -> setup initialize -> operational admin/tenant API calls
- idempotency replay semantics for setup initialize
- upgrade/redeploy preserving bootstrap state and keys

```bash
./scripts/verify_marketplace_onboarding.sh
```

Report artifact: `artifacts/marketplace_onboarding_report.json` (override with `REPORT_FILE`).

GitHub Actions manual run: `.github/workflows/marketplace-onboarding.yml`.

## Policy Engine

### Action Classification

Tool calls are automatically classified into action types based on HTTP method, path, query, and headers. Each action type is assigned a risk level (LOW, MEDIUM, HIGH, CRITICAL). Per-tenant risk overrides can adjust the default risk.

### OPA/Rego Policies

Per-tenant policies are authored in Rego and stored in PostgreSQL. The gateway evaluates policies on every tool call and produces a decision: ALLOW, DENY, or REQUIRE_APPROVAL.

- **Policy Versioning**: multiple versions per tenant with publish/rollback
- **Policy Simulation**: dry-run evaluation against test inputs via `POST /admin/tenants/{tenant_id}/policies/simulate`

### Mandatory Base Policy (Epic 11)

A system-level base policy evaluates before every tenant policy. Base policy DENY/REQUIRE_APPROVAL cannot be overridden by tenant policies:

- All CRITICAL risk actions require approval
- Destructive actions (`DATA.DELETE`, `DATA.BULK_EXPORT`) at HIGH/CRITICAL require approval
- Identity/access actions (`ACCESS.GRANT`, `ACCESS.ROLE_CHANGE`) always require approval

## Approval Workflow

When a policy returns REQUIRE_APPROVAL, the gateway creates a pending approval request with a cryptographically secure approval token. The agent receives HTTP 409 with the `approval_request_id` and `approval_token`.

- Default TTL: 15 minutes (configurable per-system via `default_approval_ttl_seconds`)
- States: PENDING, APPROVED, DENIED, EXECUTING, EXECUTED, EXPIRED, REVOKED
- Replay: agent resubmits with `X-Approval-Request-Id` and `X-Approval-Token` headers
- Retry window: 60 seconds for transient execution failures

## Security Hardening (Epic 11)

### Ephemeral Session Tokens

Short-lived HMAC-SHA256 session tokens replace static tenant keys for MCP sessions:

- Issued during `initialize` handshake
- Bound to tenant, agent, and source IP
- 15-minute TTL (configurable via `RBTR_SESSION_TOKEN_TTL_SECONDS`)
- Revocable per-session or per-tenant
- Enable: `RBTR_FEATURE_SESSION_TOKENS=true`

### File Access Governance

File path detection in tool call arguments with tenant-scoped sandbox enforcement:

- Recursive JSON argument walking detects file paths
- Path traversal (`../`) blocked by raw segment inspection
- Paths outside tenant sandbox (`/data/tenants/{tenant_id}/`) denied
- Enable: `RBTR_FEATURE_FILE_GOVERNANCE=true`

### Cross-Tenant Provenance Chain

Signed provenance tokens track agent-to-agent request chains across tenant boundaries:

- `X-Provenance-Chain` header carries HMAC-signed token (30s TTL)
- Policy input enriched with `source_tenant_id` and `chain_depth`
- ADR linkage via `source_decision_id` for full chain traceability
- Configurable max chain depth (default 5) prevents infinite loops
- Enable: `RBTR_FEATURE_CROSS_TENANT_CHAIN=true`

## Freemium Model & Tier System

rbitr operates as a freemium self-hosted product. Installations start on the free tier with constrained but functional limits. Upgrading unlocks the full feature set.

### Tier Comparison

| Dimension | Free Tier | Paid Tier |
|-----------|-----------|-----------|
| Tenants | 1 | Unlimited |
| Agents per tenant | 1 | Unlimited |
| Active tenant keys | 1 | Unlimited |
| Governed actions / month | 10,000 | Unlimited |
| Audit log retention | 7 days | 90 days (configurable up to 1 year) |
| Approval workflows | Not available | Full access |
| Evidence export | Not available | Full access |
| Integrations (Slack, Jira, etc.) | Not available | All channels |
| Custom OPA policies | Not available | Full access |

### Free Trial (14 Days)

New installations include a **14-day free trial** that automatically starts when you create your first tenant. During the trial:
- ✅ All premium features are unlocked (approval workflows, integrations, evidence export)
- ✅ No license key required
- ✅ Full feature evaluation before purchasing

After the trial expires, premium features are locked until you upload a license key. The trial is **application-wide** (not per-tenant) and based on the earliest tenant creation date.

**Trial Status:** Check your trial countdown in the UI at Settings > License or via `GET /admin/license`.

### Usage Metering

Governed actions are metered per-tenant per-month. When the monthly action limit is reached, the gateway returns `429 Too Many Requests` with an upgrade prompt. Paid tier skips metering entirely (unlimited).

Usage data is stored in the `usage_meters` table (migration `00034`). Counters are incremented atomically via PostgreSQL `ON CONFLICT ... DO UPDATE`.

### Provisioning Limits

Free-tier installations enforce resource creation limits:

| Resource | Free Limit | Error |
|----------|-----------|-------|
| Tenants | 1 | `403 Tenant limit reached` |
| Agents per tenant | 1 | `403 Agent limit reached` |
| Active keys per tenant | 1 | `403 Active key limit reached` |

Limits are checked before creation and return clear error messages with upgrade prompts.

### Feature Gating

Advanced features are gated behind entitlements. On the API side, gated endpoints return `403` with `FEATURE_NOT_AVAILABLE` when the entitlement is missing. On the UI side, gated features render with reduced opacity, a lock icon, and a tooltip CTA linking to the license settings page.

Gated route groups:

| Feature | Gated Routes | Entitlement |
|---------|-------------|-------------|
| Approval workflows | `/admin/tenants/:id/approvals/*` | `approval_workflows` |
| Evidence export | `/admin/tenants/:id/audit/export` | `evidence_export` |
| Integrations | `/admin/tenants/:id/notifications/*`, `/admin/tenants/:id/ticketing/*` | `integrations` |
| Custom OPA policies | `/admin/tenants/:id/policies/*`, `/admin/tenants/:id/risk-overrides/*` | `custom_policies` |

### Usage Dashboard

The admin API exposes usage visibility endpoints:

- `GET /admin/usage` — current period usage summary (governed actions, tenant count, feature availability, audit retention)
- `GET /admin/usage/history?months=N` — historical usage aggregated across tenants (default 6 months, max 24)

The UI dashboard shows progress bars with color-coded thresholds (green < 80%, amber 80–95%, red > 95%) and warning banners at 80% and 95% quota usage.

## Rate Limiting

Per-request rate limiting with configurable scopes and windows:

- **Scopes**: `tenant`, `tenant_agent`, `tenant_tool`, `tenant_agent_tool`
- **Windows**: per-minute and per-day limits
- **Defaults**: 60 requests/minute, 10,000 requests/day
- Returns HTTP 429 with `retry_after_seconds` when exceeded
- Enable: `RBTR_FEATURE_RATE_LIMITING=true`

Configurable via `PUT /admin/settings/default-rate-limit`.

## Argument Constraints

Policy-level constraints on tool call arguments:

- **Path-based matching**: JSONPath-style paths (e.g., `$.user_id`)
- **Operators**: `eq`, `prefix`, `regex`, `in`, `contains`, `jsonschema`
- **Allow/deny rules**: whitelist and blacklist constraints
- **Custom messages**: per-rule violation messages
- Enable: `RBTR_FEATURE_ARG_CONSTRAINTS=true`

## MCP (Model Context Protocol)

Native MCP support for AI agent frameworks:

- `POST /v1/mcp/{tenant_id}` - JSON-RPC 2.0 endpoint
- `GET /v1/mcp/{tenant_id}` - Streamable HTTP (SSE transport)

Implemented methods:

| Method | Description |
|--------|-------------|
| `initialize` | Agent handshake with session token issuance |
| `tools/list` | List available tools for the tenant |
| `tools/call` | Call tool with full governance evaluation |
| `notifications/initialized` | Agent notification acknowledgement |

Unknown methods are forwarded to the upstream MCP server without governance (configurable via `MCPPassthroughUpstreamToolID`).

Protocol: JSON-RPC 2.0, version `20251125`, max request 256 KB, SSE heartbeat every 15s.

## Authentication

### Tenant Authentication

- `Authorization: Bearer <tenant_key>` (preferred)
- `X-Tenant-Key` header (legacy fallback)
- HMAC-SHA256 key verification with automatic hash algorithm upgrade
- Agent ID required via `X-Agent-Id` header
- Key rotation: `RBTR_TENANT_KEY_HMAC_SECRETS` (comma-separated current, previous...)

### Admin Authentication

- `Authorization: Bearer <admin_key>` (preferred)
- `X-Admin-Key` header (legacy fallback)
- Scope-based access control (25 granular scopes)
- Key rotation: `RBTR_ADMIN_KEY_HMAC_SECRETS` (comma-separated current, previous...)

### SSO / OIDC Authentication (Epic 12)

Admin console supports SSO via any OIDC-compliant identity provider. Dual auth: SSO sessions and API keys work side-by-side.

| Provider | Issuer URL Example |
|----------|-------------------|
| Google Workspace | `https://accounts.google.com` |
| AWS IAM Identity Center | `https://your-sso-portal.awsapps.com/start` |
| Okta | `https://your-org.okta.com` |
| Azure AD / Entra ID | `https://login.microsoftonline.com/{tenant-id}/v2.0` |
| Auth0 | `https://your-domain.auth0.com` |
| Keycloak | `https://keycloak.example.com/realms/{realm}` |

| Env Var | Description |
|---------|-------------|
| `RBTR_SSO_ENABLED` | Enable SSO (`true`/`false`) |
| `RBTR_SSO_ISSUER` | OIDC issuer URL |
| `RBTR_SSO_CLIENT_ID` | OAuth2 client ID |
| `RBTR_SSO_CLIENT_SECRET_REF` | Secret ref for client secret (e.g., `env://SSO_CLIENT_SECRET`) |
| `RBTR_SSO_REDIRECT_URI` | OAuth2 callback URL |
| `RBTR_SSO_ALLOWED_DOMAINS` | Comma-separated allowed email domains |
| `RBTR_SSO_DEFAULT_SCOPES` | Default admin scopes (default: `admin:read,admin:write`) |

SSO can also be configured at runtime via the admin Settings UI. The login page automatically shows a "Sign in with SSO" button when SSO is enabled.

### Admin Scopes

| Scope | Description |
|-------|-------------|
| `admin:tenants:read` | List and read tenant details |
| `admin:tenants:write` | Update tenant config, create/delete tenants |
| `admin:keys:read` | List tenant keys |
| `admin:keys:rotate` | Rotate tenant keys |
| `admin:keys:revoke` | Revoke tenant keys |
| `admin:tools:read` | List and read tool definitions |
| `admin:tools:write` | Update tool config and metadata |
| `admin:policies:read` | View policies |
| `admin:policies:write` | Create and update policies |
| `admin:policies:publish` | Publish policy versions |
| `admin:policies:rollback` | Rollback to previous policy |
| `admin:policies:simulate` | Simulate policy decisions |
| `admin:approvals:read` | View approval requests |
| `admin:approvals:decide` | Approve, deny, or revoke approvals |
| `admin:audit:read` | Read audit trail |
| `admin:audit:export` | Export audit trail (JSON/CSV) |
| `admin:notifications:read` | View notification config |
| `admin:notifications:write` | Update notification channels |
| `admin:notifications:test` | Send test notifications |
| `admin:ticketing:read` | View ticketing config and links |
| `admin:ticketing:write` | Update ticketing config |
| `admin:ticketing:test` | Create test tickets |
| `admin:settings:read` | Read system settings |
| `admin:settings:write` | Modify system settings |

Umbrella scopes: `admin:read` covers all `:read`/`:export`/`:simulate` scopes. `admin:write` covers all other scopes.

## Tenant Management

Multi-tenant architecture with per-tenant isolation:

- **Create/delete tenants**: `POST /admin/tenants`, `DELETE /admin/tenants/{tenant_id}`
- **Enable/disable**: `PUT /admin/tenants/{tenant_id}/enabled`
- **Key lifecycle**: create, rotate, and revoke tenant API keys
- **Per-tenant config**: policies, tools, notifications, rate limits, enforcement mode

Each tenant has isolated:
- Tool definitions with independent backend URLs and auth
- OPA/Rego policies with version history
- Notification channel configuration
- Risk overrides
- Audit trail

### Enforcement Mode

Per-tenant enforcement mode controls whether decisions are enforced or reported:

- `enforce` (default) - DENY and REQUIRE_APPROVAL decisions are enforced
- `shadow` - all decisions are recorded but not enforced (tool calls always proceed)

## Notifications (Epic 12)

rbitr supports multi-channel notifications for approval events, security alerts, and policy violations.

### Channels

| Channel | Config Key | Secret Ref |
|---------|-----------|------------|
| Slack Webhook | `slack_webhook_enabled` | `env://` or `file://` |
| Slack Bot | `slack_bot_enabled` | `env://` or `file://` |
| Email (SES/SendGrid/Mailgun) | `email_enabled` | `env://` or `file://` |
| Telegram | `telegram_enabled` | `env://` or `file://` |
| WhatsApp Business | `whatsapp_enabled` | `env://` or `file://` |

All channels are configured per-tenant via the admin API and UI (Notifications page). Secret refs support all registered secret providers (see below).

### Notification Events

| Event | Severity | Description |
|-------|----------|-------------|
| `APPROVAL.EXPIRING` | WARN | Approval expires in ~1 minute |
| `APPROVAL.EXPIRED` | INFO | Approval has expired |
| `SECURITY.TOKEN_ABUSE` | CRITICAL | Approval token abuse detected |
| `POLICY.INVALID_OUTPUT` | WARN | Policy returned malformed decision |
| `POLICY.EVAL_ERROR` | WARN | Policy evaluation runtime error |

Per-event enablement toggles, suppression tracking (dedup + cooldown), and mailing list support for email distribution.

### Ticketing & ITSM Integration (Epic 12)

Bidirectional integration with enterprise ticketing systems. When an approval is required, rbitr auto-creates a ticket; when an operator resolves the ticket, a webhook triggers approval in rbitr.

| Provider | API | Protocol |
|----------|-----|----------|
| Jira | REST API v3 | HTTP + Basic Auth or Bearer |
| ServiceNow | REST Table API | HTTP + Bearer |
| Linear | GraphQL API | HTTP + Bearer |

Per-tenant configuration via admin API:
- Provider, base URL, project key, issue type
- Auto-create toggle (creates tickets on REQUIRE_APPROVAL)
- API token stored as secret ref (supports all registered secret providers)
- Webhook signing secret for inbound signature verification (HMAC-SHA256)

Lifecycle:
1. Agent request triggers REQUIRE_APPROVAL → ticket auto-created in configured provider
2. Approval decision (approve/deny/revoke) → ticket updated with comment + status transition
3. Approval expiry → ticket updated and closed
4. Inbound webhook from provider (ticket resolved/closed) → rbitr auto-approves/denies the linked approval

### Secret Providers

Secret references in notification and SSO config are resolved via pluggable providers. The built-in providers (`env://`, `file://`) are always available. Cloud providers are opt-in.

| Provider | URI Scheme | Enable Env Var | Required Env Vars |
|----------|-----------|----------------|-------------------|
| Environment variable | `env://` | Always on | -- |
| File | `file://` | Always on | -- |
| AWS Secrets Manager | `aws-sm://` | `RBTR_SECRET_PROVIDER_AWS=true` | AWS credential chain (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`) |
| GCP Secret Manager | `gcp-sm://` | `RBTR_SECRET_PROVIDER_GCP=true` | `GCP_SECRET_MANAGER_TOKEN` or GCE/GKE metadata |
| HashiCorp Vault | `vault://` | `RBTR_SECRET_PROVIDER_VAULT=true` | `VAULT_ADDR`, `VAULT_TOKEN` |
| Azure Key Vault | `azure-kv://` | `RBTR_SECRET_PROVIDER_AZURE=true` | `AZURE_KEY_VAULT_TOKEN` |

Cloud providers can also be toggled from the admin UI under **Settings > Secret providers**. All resolved secrets are cached with a 5-minute TTL.

Examples:

```
env://SLACK_WEBHOOK_URL
file:///run/secrets/slack-token
aws-sm://prod/rbitr/slack-token
gcp-sm://projects/myproj/secrets/slack-token
vault://secret/data/rbitr/slack#token
azure-kv://myvault/slack-token
```

## API Reference

### Public API (Tenant-Scoped)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/tools/{tool_id}/call` | Call a tool with governance |
| `GET` | `/v1/tenants/{tenant_id}/evidence` | Tenant evidence trail |
| `POST` | `/v1/mcp/{tenant_id}` | MCP JSON-RPC endpoint |
| `GET` | `/v1/mcp/{tenant_id}` | MCP Streamable HTTP (SSE) |

### Admin API

**Tenant Management**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants` | List tenants |
| `POST` | `/admin/tenants` | Create tenant |
| `GET` | `/admin/tenants/{tenant_id}` | Tenant details |
| `DELETE` | `/admin/tenants/{tenant_id}` | Delete tenant |
| `PUT` | `/admin/tenants/{tenant_id}/config` | Update tenant name/key |
| `PUT` | `/admin/tenants/{tenant_id}/enabled` | Enable/disable tenant |
| `GET` | `/admin/tenants/{tenant_id}/keys` | List tenant keys |
| `POST` | `/admin/tenants/{tenant_id}/keys` | Create key |
| `POST` | `/admin/tenants/{tenant_id}/keys/rotate` | Rotate keys |
| `POST` | `/admin/tenants/{tenant_id}/keys/{key_id}/revoke` | Revoke key |

**Tools**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/tools` | List tools |
| `PUT` | `/admin/tenants/{tenant_id}/tools/{tool_id}` | Update tool config |
| `PUT` | `/admin/tenants/{tenant_id}/tools/{tool_id}/metadata` | Update tool metadata |

**Policies**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/policies` | List policy versions |
| `GET` | `/admin/tenants/{tenant_id}/policies/{version}` | Get policy |
| `POST` | `/admin/tenants/{tenant_id}/policies` | Create policy version |
| `POST` | `/admin/tenants/{tenant_id}/policies/simulate` | Simulate policy |
| `PUT` | `/admin/tenants/{tenant_id}/policies/{version}/publish` | Publish version |
| `PUT` | `/admin/tenants/{tenant_id}/policies/rollback` | Rollback policy |

**Approvals**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/approvals` | List approvals |
| `GET` | `/admin/tenants/{tenant_id}/approvals/{id}` | Approval details |
| `GET` | `/admin/tenants/{tenant_id}/approvals/pending-count` | Pending count |
| `POST` | `/admin/tenants/{tenant_id}/approvals/{id}/approve` | Approve |
| `POST` | `/admin/tenants/{tenant_id}/approvals/{id}/deny` | Deny |
| `POST` | `/admin/tenants/{tenant_id}/approvals/{id}/revoke` | Revoke |

**Audit**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/audit` | Tenant audit trail |
| `GET` | `/admin/tenants/{tenant_id}/audit/export` | Export audit (JSON/CSV) |
| `GET` | `/admin/tenants/{tenant_id}/audit/resource-types` | Resource types |
| `GET` | `/admin/audit` | Global audit trail |

**Notifications**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/notifications` | Get config |
| `PUT` | `/admin/tenants/{tenant_id}/notifications` | Update config |
| `PUT` | `/admin/tenants/{tenant_id}/notifications/{channel}-secret-ref` | Set channel secret |
| `POST` | `/admin/tenants/{tenant_id}/notifications/test/{channel}` | Test send |
| `GET` | `/admin/tenants/{tenant_id}/notifications/suppressions` | Suppression history |
| `GET/POST/PUT/DELETE` | `/admin/tenants/{tenant_id}/mailing-lists/...` | Mailing list CRUD |

**Ticketing**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/ticketing` | Get ticketing config |
| `PUT` | `/admin/tenants/{tenant_id}/ticketing` | Update ticketing config |
| `PUT` | `/admin/tenants/{tenant_id}/ticketing/secret-ref` | Set API token secret |
| `PUT` | `/admin/tenants/{tenant_id}/ticketing/webhook-secret-ref` | Set webhook signing secret |
| `POST` | `/admin/tenants/{tenant_id}/ticketing/test` | Create test ticket |
| `GET` | `/admin/tenants/{tenant_id}/ticketing/links` | List ticket links |
| `POST` | `/admin/webhooks/ticketing/{provider}` | Inbound webhook receiver |

**Risk Overrides**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/tenants/{tenant_id}/risk-overrides` | List overrides |
| `PUT` | `/admin/tenants/{tenant_id}/risk-overrides/{action_type}` | Set override |
| `DELETE` | `/admin/tenants/{tenant_id}/risk-overrides/{action_type}` | Remove override |

**System Settings**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/settings` | Get all settings |
| `GET` | `/admin/me` | Current admin info |
| `GET` | `/admin/action-types` | Available action types |
| `PUT` | `/admin/settings/default-approval-ttl` | Approval TTL |
| `PUT` | `/admin/settings/default-rate-limit` | Rate limit config |
| `PUT` | `/admin/settings/audit-retention` | Audit retention days |
| `PUT` | `/admin/settings/enforcement-mode` | Enforcement mode |
| `PUT` | `/admin/settings/feature-*` | Feature flag toggles |
| `PUT` | `/admin/settings/secret-provider-*` | Secret provider toggles |
| `PUT` | `/admin/settings/sso-enabled` | Toggle SSO |
| `PUT` | `/admin/settings/sso-config` | SSO configuration |

**Entitlements & Usage**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/license/entitlements` | Current tier and feature entitlements |
| `GET` | `/admin/usage` | Current period usage summary |
| `GET` | `/admin/usage/history` | Historical usage (default 6 months, max 24) |

**SSO**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/auth/sso/status` | SSO enabled status (no auth) |
| `GET` | `/admin/auth/sso/config` | SSO configuration (admin auth) |
| `GET` | `/admin/auth/sso/authorize` | Start OIDC flow (no auth) |
| `GET` | `/admin/auth/sso/callback` | OIDC callback (no auth) |
| `POST` | `/admin/auth/sso/logout` | Revoke SSO session |

## Observability

### Structured Logging

Structured request logging via Echo/v5 with context fields: `request_id`, `tenant_id`, `agent_id`, `tool_id`, `action_type`, `decision`, `latency_ms`.

### Prometheus Metrics

Gateway metrics exposed at `GET /metrics`:

- `decisions_total{decision,action_type}` - governance decisions
- `gateway_requests_total` - total requests
- `tool_exec_total` - tool executions
- `errors_total` - errors
- `decision_latency_ms` - policy evaluation latency
- `tool_latency_ms` - tool execution latency
- `rate_limit_latency_ms` - rate limit check latency
- `policy_eval_invalid_total{reason}` - invalid policy outputs
- `approvals_created_total` - approvals created
- `approvals_executed_total{result}` - approval executions
- `notifications_sent_total{channel,event_type}` - notifications sent
- `notifications_suppressed_total{channel,event_type}` - suppressed notifications
- `cache_hits_total{cache_name}` / `cache_misses_total{cache_name}` - cache stats
- `setup_attempts_total{result}` / `setup_duration_ms` / `setup_state{state}`

### Health Checks

- `GET /healthz` - liveness probe (always 200)
- `GET /readyz` - readiness probe (checks DB connectivity)

### Audit Trail (SOC-ready)

Audit events are append-only and immutable. Each event is chained with a per-tenant hash:

- `prev_hash` + canonical JSON payload -> `event_hash`
- `stream_id` is the tenant id (or `global` for non-tenant events)

Retention is configurable via settings:

- `audit_retention_days` (default 365)
- Cleanup runs daily inside the gateway with an advisory lock.

Export endpoints (tenant-scoped):

- `GET /admin/tenants/{tenant_id}/audit/export?format=json|csv`
- `include_details=true` to include `before/after` payloads

Export defaults are safe-by-default (redacted payloads, no secrets).

## Feature Flags

All features can be toggled via environment variables at startup or via the admin Settings UI at runtime.

| Flag | Default | Description |
|------|---------|-------------|
| `RBTR_FEATURE_RATE_LIMITING` | `false` | Per-request rate limiting |
| `RBTR_FEATURE_ARG_CONSTRAINTS` | `false` | Argument constraint enforcement |
| `RBTR_FEATURE_SESSION_TOKENS` | `true` | Ephemeral MCP session tokens |
| `RBTR_FEATURE_FILE_GOVERNANCE` | `true` | File path sandbox enforcement |
| `RBTR_FEATURE_CROSS_TENANT_CHAIN` | `false` | Cross-tenant provenance tracking |
| `RBTR_SSO_ENABLED` | `false` | OIDC SSO for admin console |
| `RBTR_SECRET_PROVIDER_AWS` | `false` | AWS Secrets Manager |
| `RBTR_SECRET_PROVIDER_GCP` | `false` | GCP Secret Manager |
| `RBTR_SECRET_PROVIDER_VAULT` | `false` | HashiCorp Vault |
| `RBTR_SECRET_PROVIDER_AZURE` | `false` | Azure Key Vault |

## Demo

### One-shot script

`./scripts/test_demo.sh` exercises the full workflow (ALLOW, REQUIRE_APPROVAL with admin approval, DENY, and evidence trail). It self-bootstraps with its own defaults (`t_acme`, `rbtr_live_...`, `rbtr_admin_...`) and assumes the gateway and mocktool are already running.

```bash
# terminal 1
go run ./cmd/mocktool
# terminal 2
go run ./cmd/gateway
# terminal 3
./scripts/test_demo.sh
```

### Manual walkthrough

Demo defaults (from `./scripts/dev/bootstrap.sh`):

- tenant id: `t_demo`
- admin key: `admin_demo_key_2026!`
- tenant key: `tenant_demo_key_2026!`

1. Start services (two terminals):

```bash
go run ./cmd/mocktool
go run ./cmd/gateway
```

2. Bootstrap the tenant once the gateway is up:

```bash
./scripts/dev/bootstrap.sh
```

3. Allowed call (DATA.READ):

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo" \
-d '{
"http_method": "GET",
"path": "/status",
"query": "",
"headers": {"Accept": "application/json"},
"body": ""
}'
```

Expect: decision `ALLOW`.

4. Approval required (PAYMENT.REFUND):

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo" \
-d '{
"http_method": "POST",
"path": "/refund",
"query": "",
"headers": {"Content-Type": "application/json"},
"body": "{\"amount\":100}"
}'
```

Expect: HTTP 409 with `approval_request_id` + `approval_token`. Clients should treat this as pending approval and replay after approval.

4b. Admin approves:

```bash
curl -sS -X POST "http://localhost:8080/admin/tenants/t_demo/approvals/<approval_request_id>/approve" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer admin_demo_key_2026!" \
-d '{"comment":"approved in demo"}'
```

4c. Agent resubmits with approval headers:

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo" \
-H "X-Approval-Request-Id: <approval_request_id>" \
-H "X-Approval-Token: <approval_token>" \
-d '{
"http_method": "POST",
"path": "/refund",
"query": "",
"headers": {"Content-Type": "application/json"},
"body": "{\"amount\":100}"
}'
```

Expect: HTTP 200 with tool response and approval marked EXECUTED.

5. Denied (DATA.EXPORT):

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo" \
-d '{
"http_method": "POST",
"path": "/export_customer_data",
"query": "",
"headers": {"Content-Type": "application/json"},
"body": "{}"
}'
```

Expect: HTTP 403 with decision `DENY`.

6. Evidence trail:

```bash
curl -sS "http://localhost:8080/v1/tenants/t_demo/evidence?limit=50" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo"
```
