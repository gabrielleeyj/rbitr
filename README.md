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

## Prometheus Metrics

- `decisions_total{decision,action_type}`
- `gateway_requests_total`
- `tool_exec_total`
- `errors_total`
- `decision_latency_ms`

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
