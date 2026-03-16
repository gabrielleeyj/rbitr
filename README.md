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

### Option A: Docker Compose

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

- Gateway: http://localhost:8080
- UI: http://localhost:5173
- Postgres: localhost:2345

Bootstrap first tenant/admin keys:

```bash
./scripts/dev/bootstrap.sh
```

### Option B: Local Binaries

1. Run migrations:

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:2345/rbitr?sslmode=disable
goose -dir migrations postgres "$DATABASE_URL" up
```

2. Start mock tool and gateway:

```bash
go run ./cmd/mocktool
go run ./cmd/gateway
```

3. Run tests: `go test ./...`

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

## Security Hardening (Epic 11)

### Mandatory Base Policy

A system-level base policy evaluates before every tenant policy. Base policy DENY/REQUIRE_APPROVAL cannot be overridden by tenant policies:

- All CRITICAL risk actions require approval
- Destructive actions (`DATA.DELETE`, `DATA.BULK_EXPORT`) at HIGH/CRITICAL require approval
- Identity/access actions (`ACCESS.GRANT`, `ACCESS.ROLE_CHANGE`) always require approval

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

## API

- Tool call payload is a JSON envelope with `http_method`, `path`, `query`, `headers`, `body` expected by `POST /v1/tools/{tool_id}/call`.
- Production migrations are schema-only (no seeded demo data).
- Tenant auth: `Authorization: Bearer <tenant_key>` (preferred). `X-Tenant-Key` is legacy fallback.
- Admin auth: `Authorization: Bearer <admin_key>` (preferred). `X-Admin-Key` is legacy fallback.
- HMAC secret rotation: `RBTR_TENANT_KEY_HMAC_SECRETS` and `RBTR_ADMIN_KEY_HMAC_SECRETS` accept comma-separated values (current, previous...).

## Observability

### Structured Logging

Structured request logging via Echo/v5 with context fields: `request_id`, `tenant_id`, `agent_id`, `tool_id`, `action_type`, `decision`, `latency_ms`.

### Prometheus Metrics

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

## Demo

Run `./scripts/test_demo.sh` to test the full workflow (ALLOW, REQUIRE_APPROVAL with admin approval, DENY, and evidence trail).

Before running the demo, bootstrap setup once:

```bash
./scripts/dev/bootstrap.sh
```

Demo defaults:

- tenant id: `t_demo`
- admin key: `admin_demo_key_2026!`
- tenant key: `tenant_demo_key_2026!`

1. Start services (two terminals):

```bash
go run ./cmd/mocktool
go run ./cmd/gateway
```

2. Allowed call (DATA.READ):

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

3. Approval required (PAYMENT.REFUND):

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

3b. Admin approves:

```bash
curl -sS -X POST "http://localhost:8080/admin/tenants/t_demo/approvals/<approval_request_id>/approve" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer admin_demo_key_2026!" \
-d '{"comment":"approved in demo"}'
```

3c. Agent resubmits with approval headers:

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

4. Denied (DATA.EXPORT):

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

5. Evidence trail:

```bash
curl -sS "http://localhost:8080/v1/tenants/t_demo/evidence?limit=50" \
-H "Authorization: Bearer tenant_demo_key_2026!" \
-H "X-Agent-Id: agent_demo"
```
