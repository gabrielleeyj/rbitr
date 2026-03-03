# rbitr - the agent governance control plane.

![Unit Tests](https://github.com/gabrielleeyj/rbitr/actions/workflows/go.yml/badge.svg)

## Introduction

What this does:

Governance semantics: canonicalization + hashing + action classification

- OPA/Rego policies stored in DB (policy-as-data)
- ADR persistence (governance artifact, not access logs)
- Approval-request persistence (the “human-in-loop” hook)
- Evidence export with DTO whitelist + contract validation + redaction tests
- Risk overrides
- Metrics for decisioning and latency

Demo theme: “Enterprise customer asks: prove your AI agent can’t refund/export/change permissions without controls.”

You respond with: action policies + approval artifacts + tenant evidence pack + policy snapshot + simulation result.

What it focuses on:

- Payments/refunds
- Data export / privacy
- Access/permissions
- Support ops (ticketing + CRM updates)

## Getting Started (Dev)

### Option A: Docker compose (API + UI + Postgres + migrate)

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

- Gateway: http://localhost:8080
- UI: http://localhost:5173
- Postgres: localhost:2345

Bootstrap first tenant/admin keys (recommended defaults match demo scripts):

```bash
./scripts/dev/bootstrap.sh
```

### First-run Setup Wizard (Epic 9)

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

Wizard bootstrap sequence:

1. Validate environment readiness (DB connectivity + schema presence)
2. Create initial tenant profile
3. Create admin key and tenant key (auto-generated or user-provided)
4. Seed default policy and activate it for the tenant
5. Mark bootstrap complete

Note: the setup flow currently validates that migrations are present but does not run migrations itself; run migrations before using `/setup/initialize`.

Dev note: when `RBTR_DEV_AUTO_TOOLS=true`, setup auto-seeds per-tenant dev tool wiring:

- `mock_internal` -> `RBTR_DEV_MOCK_INTERNAL_URL` (default `http://localhost:8090`)
- `jira` -> `RBTR_DEV_JIRA_URL` (default `http://localhost:8081`)

### Marketplace Onboarding Verification Harness

Use the onboarding-aware harness to validate:

- fresh install -> setup initialize -> operational admin/tenant API calls
- idempotency replay semantics for setup initialize
- upgrade/redeploy preserving bootstrap state and keys

Run locally:

```bash
./scripts/verify_marketplace_onboarding.sh
```

Report artifact output (machine-readable JSON):

- default: `artifacts/marketplace_onboarding_report.json`
- override with `REPORT_FILE=/path/to/report.json`
- optional compose migration URL override: `COMPOSE_DATABASE_URL=postgres://...`

The script enforces token-required setup mode during verification (`RBTR_SETUP_TOKEN_REQUIRED=true`) and checks:

- `Authorization: Bearer <setup_token>`
- `Idempotency-Key` requirement and replay behavior

GitHub Actions manual run is available via workflow:

- `.github/workflows/marketplace-onboarding.yml`
- uploaded artifact name: `marketplace-onboarding-report`

### Release Pipeline

Release CI/CD is defined in:

- `.github/workflows/release.yml`

Release triggers:

- push tag `v*` (for example `v1.2.3`)
- manual `workflow_dispatch` with an existing release tag

Release gates:

- `golangci-lint`
- `go test ./...`
- setup smoke gate (`scripts/test_setup_smoke.sh`)
- marketplace onboarding harness (`scripts/verify_marketplace_onboarding.sh`)

Release outputs:

- platform binary archives + `SHA256SUMS`
- onboarding verification report artifact
- GHCR gateway/mocktool multi-arch images (optional for manual runs)
- published GitHub Release with attached artifacts

### Database runtime defaults

- If `DATABASE_URL` is not set, the gateway defaults to:
  - `postgres://postgres@localhost:2345/rbitr?sslmode=require`
- Connection pool defaults (overridable via env):
  - `DB_MAX_OPEN_CONNS=30`
  - `DB_MAX_IDLE_CONNS=10`
  - `DB_CONN_MAX_LIFETIME_SECONDS=1800`
  - `DB_CONN_MAX_IDLE_TIME_SECONDS=300`

### Option B: Local binaries

1. Run migrations:

```bash
# Local dev Postgres in this repo runs without TLS, so sslmode=disable is explicit here.
export DATABASE_URL=postgres://postgres:postgres@localhost:2345/rbitr?sslmode=disable
goose -dir migrations postgres "$DATABASE_URL" up
```

2. Start mock tool and gateway:

```bash
go run ./cmd/mocktool
go run ./cmd/gateway
```

3. Run tests: `go test ./...`

## Information

- Tool call payload is a JSON envelope with:
  1. `http_method`,
  2. `path`,
  3. `query`,
  4. `headers`,
  5. `body`
     (string) expected by `POST /v1/tools/{tool_id}/call`.
- Production migrations are schema-only (no seeded demo tenant/keys/tools).
- For local demo defaults, run `./scripts/dev/bootstrap.sh`, which initializes:
  - tenant id `t_demo`
  - admin key `admin_demo_key_2026!`
  - tenant key `tenant_demo_key_2026!`

- Admin writes are allowed post-bootstrap unless `admin_write_lock` is enabled.
- Tenant auth accepts `Authorization: Bearer <tenant_key>` (preferred). `X-Tenant-Key` is legacy fallback and may be disabled.
- Admin auth accepts `Authorization: Bearer <admin_key>` (preferred) or `X-Admin-Key` (legacy).
- Tenant key hashing supports HMAC secret rotation via `RBTR_TENANT_KEY_HMAC_SECRETS` (comma-separated: current,previous...).
- Admin key hashing supports HMAC secret rotation via `RBTR_ADMIN_KEY_HMAC_SECRETS` (comma-separated: current,previous...).

## Structured Logging

Implemented structured request logging using Echo/v5 request logger and wired context fields so logs include `request_id`, `tenant_id`, `agent_id`,
`tool_id`, `action_type`, `decision`, and `latency_ms`.

Additional Fields (TBD):

- policy version
- rule_id
- request_hash

## Prometheus Metrics

- `decisions_total{decision,action_type}`
- `gateway_requests_total`
- `tool_exec_total`
- `errors_total`
- `decision_latency_ms`
- `tool_latency_ms`
- `policy_eval_invalid_total{reason}`
- `setup_attempts_total{result}`
- `setup_duration_ms`
- `setup_state{state}`

## Audit Trail (SOC-ready)

Admin audit events are append-only and immutable. Each event is chained with a per-tenant hash:

- `prev_hash` + canonical JSON payload -> `event_hash`
- `stream_id` is the tenant id (or `global` for non-tenant events)

Retention is configurable via settings:

- `audit_retention_days` (default 365)
- Cleanup runs daily inside the gateway with an advisory lock.

Export endpoints (tenant-scoped):

- `GET /admin/tenants/{tenant_id}/audit/export?format=json|csv`
- `include_details=true` to include `before/after` payloads

Export defaults are safe-by-default (redacted payloads, no secrets).

## Simulation

**Quick test**: Run `./scripts/test_demo.sh` to test the full workflow (ALLOW, REQUIRE_APPROVAL with admin approval, DENY, and evidence trail).

Before running the demo test, bootstrap setup once:

```bash
./scripts/dev/bootstrap.sh
```

1. Start services (two terminals)

go run ./cmd/mocktool

go run ./cmd/gateway

2. Allowed call (generic DATA.READ against mock_internal)

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

Expect: decision: "ALLOW" and tool_status likely 404 (mock
tool doesn’t implement /status), but ADR is still recorded.

3. Approval required (PAYMENT.REFUND)

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

Expect: HTTP 409 with approval_request_id + approval_token.
This is an expected execution gate (not a terminal failure): clients should treat it as pending approval and replay the same request after approval with `X-Approval-Request-Id` and `X-Approval-Token`.

3b. Admin approves the request

```bash
curl -sS -X POST "http://localhost:8080/admin/tenants/t_demo/approvals/<approval_request_id>/approve" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer admin_demo_key_2026!" \
-d '{"comment":"approved in demo"}'
```

3c. Agent resubmits with approval headers

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

4. Denied (DATA.EXPORT)

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

**Expect**: HTTP 403 with decision: "DENY".

5. Evidence preview (should show all ADRs)

```bash
curl -sS "http://localhost:8080/v1/tenants/t_demo/evidence?limit=50" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo"
```

If you want a 200 “ALLOW” response body, we can add a /status
handler in cmd/mocktool or point the jira tool to a mock
server.
