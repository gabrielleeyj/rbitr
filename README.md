# rbitr - the agent governance control plane.

## Introduction

What this does:

Governance semantics: canonicalization + hashing + action classification ✅
OPA/Rego policies stored in DB ✅ (policy-as-data)
ADR persistence ✅ (governance artifact, not access logs)
Approval-request persistence ✅ (the “human-in-loop” hook)
Evidence export with DTO whitelist + contract validation + redaction tests ✅
Risk overrides ✅ and you already made them editable post-bootstrap (good call)
Metrics for decisioning and latency ✅

Demo theme: “Enterprise customer asks: prove your AI agent can’t refund/export/change permissions without controls.”

You respond with: action policies + approval artifacts + tenant evidence pack + policy snapshot + simulation result.

What it focuses on:

- Payments/refunds
- Data export / privacy
- Access/permissions
- Support ops (ticketing + CRM updates)

## Getting Started (Dev)

1. Run migrations:

```bash
export DATABASE_URL=postgres://postgres@localhost:2345/rbitr?sslmode=disable \
goose -dir migrations postgres "$DATABASE_URL" up
```

2. Start mock tool and gateway: go run `./cmd/mocktool` and `go run ./cmd/gateway`
3. Run tests: `go test ./...`

## Information

- Tool call payload is a JSON envelope with:
  1. `http_method`,
  2. `path`,
  3. `query`,
  4. `headers`,
  5. `body`
     (string) expected by `POST /v1/tools/{tool_id}/call`.
- Seeded tenant `t_demo` with

  ```
  X-Tenant-Key = tenant_demo_key;
  admin key = `admin_demo_key`;
  ```

  - tools mock_internal (`http://localhost:8090`)
  - jira (`http://localhost:8081`) in `migrations/00001_init.sql`.

- Bootstrap update endpoints are locked after `PUT /admin/bootstrap/complete`.

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

## Simulation

1. Start services (two terminals)

go run ./cmd/mocktool

go run ./cmd/gateway

2. Allowed call (generic DATA.READ against mock_internal)

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "X-Tenant-Key: tenant_demo_key" \
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
-H "X-Tenant-Key: tenant_demo_key" \
-H "X-Agent-Id: agent_demo" \
-d '{
"http_method": "POST",
"path": "/refund",
"query": "",
"headers": {"Content-Type": "application/json"},
"body": "{\"amount\":100}"
}'
```

Expect: HTTP 409 with approval_request_id.

4. Denied (DATA.EXPORT)

```bash
curl -sS -X POST "http://localhost:8080/v1/tools/mock_internal/call" \
-H "Content-Type: application/json" \
-H "X-Tenant-Key: tenant_demo_key" \
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
-H "X-Tenant-Key: tenant_demo_key" \
-H "X-Agent-Id: agent_demo"
```

If you want a 200 “ALLOW” response body, we can add a /status
handler in cmd/mocktool or point the jira tool to a mock
server.
