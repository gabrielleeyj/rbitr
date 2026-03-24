# Incident Response Runbook

This runbook covers incident classification, detection, triage, and postmortem procedures for rbitr.

## Severity Classification

| Severity | Description | Response Time | Examples |
|----------|-------------|---------------|----------|
| SEV1 | Service fully down | Immediate | Gateway unreachable, database offline |
| SEV2 | Degraded / elevated errors | < 30 min | High error rate, decision latency spikes |
| SEV3 | Non-critical component failure | < 4 hours | Notification delivery failure, approval expiry scheduler stalled |
| SEV4 | Informational | Next business day | Cache hit rate drop, deprecated API usage |

## Detection & Alerting

### Health Endpoints

| Endpoint | Purpose | Failure Meaning |
|----------|---------|-----------------|
| `GET /healthz` | Liveness probe | Gateway process is unresponsive |
| `GET /readyz` | Readiness probe | Database is unreachable |

### Key Prometheus Metrics

Configure alerts on these metrics (exposed at `GET /metrics`):

| Metric | Type | Alert Condition |
|--------|------|-----------------|
| `gateway_requests_total` | Counter | Sudden drop → traffic loss |
| `errors_total` | Counter | Spike above baseline → elevated error rate |
| `decision_latency_ms` | Histogram | p99 > threshold → policy eval slowdown |
| `tool_latency_ms` | Histogram | p99 > threshold → upstream tool timeout |
| `rate_limit_exceeded_total` | Counter (labels: `window`, `scope`) | Spike → legitimate traffic being blocked |
| `policy_eval_invalid_total` | Counter (label: `reason`) | Any increase → policy compilation/eval error |
| `approvals_resolved_total` | Counter (label: `resolution`) | Increase in `expired` → approval expiry issue |
| `notifications_sent_total` | Counter (labels: `channel`, `event_type`, `result`) | Increase in `result=error` → notification delivery failure |
| `tenant_auth_fallback_total` | Counter | Any increase → deprecated X-Tenant-Key header in use |
| `tenant_key_legacy_upgrade_total` | Counter | Any increase → legacy key hash being upgraded |

### Log Monitoring

The gateway emits structured logs via Echo's request logger middleware. Key log patterns:

- `"db connect failed"` — startup database connection failure
- `"audit retention cleanup failed"` — retention scheduler error
- `"audit retention lock failed"` — advisory lock contention

## Triage Decision Tree

### Gateway Unreachable (SEV1)

1. Check container/pod status: `docker ps` or `kubectl get pods`.
2. Check `/healthz` — if no response, gateway process is down. Restart it.
3. Check `/readyz` — if returns `503`, database is unreachable. Proceed to "Database Unreachable."
4. Check logs for `"failed to start server"` or port binding errors.

### Database Unreachable (SEV1/SEV2)

1. Verify database connectivity: `pg_isready -h <host> -p <port>`.
2. Check connection pool exhaustion — if `DB_MAX_OPEN_CONNS` (default: `30`) is too low for load, increase it.
3. Check PostgreSQL logs for `"too many connections"` or lock contention.
4. If cloud-managed: check AWS RDS/GCP Cloud SQL console for instance status.

### High Error Rate (SEV2)

1. Check `errors_total` rate — identify when the spike started.
2. Check `decision_latency_ms` — if latency is high, the policy engine may be slow.
3. Check `tool_latency_ms` — if high, upstream tools may be timing out (request timeout: 15s).
4. Check application logs for specific error messages.

### Policy Evaluation Failures (SEV2/SEV3)

1. Check `policy_eval_invalid_total` — the `reason` label indicates the failure type.
2. Common reasons: Rego syntax error, missing data, incompatible policy version.
3. Use `POST /admin/tenants/:tenant_id/policies/simulate` to test the policy.
4. If needed, roll back: `PUT /admin/tenants/:tenant_id/policies/rollback`.

### Approval Expiry Issues (SEV3)

1. Check `approvals_resolved_total{resolution="expired"}` — if spiking, approvals are expiring before review.
2. Verify the approval expiry scheduler is running (started in `cmd/gateway/main.go`, checks every 1 minute, 5-minute expiry window).
3. Check if the default approval TTL is appropriate: `GET /admin/settings`.
4. Check notification delivery — approvals may not be reaching reviewers.

### Notification Delivery Failure (SEV3)

1. Check `notifications_sent_total{result="error"}` — identify channel (`slack`, `email`).
2. Check `notifications_latency_ms` — high latency may indicate upstream timeout.
3. Verify secret references resolve: secrets use `env://` or `file://` URI scheme.
4. Test delivery: `POST /admin/tenants/:tenant_id/notifications/test/slack` or `/test/email`.
5. Check `notifications_suppressed_total` — suppression may be throttling via cooldown (default: 10 minutes).

### Usage Quota Exceeded (SEV3/SEV4)

Free-tier installations have a 10,000 governed actions/month limit. When exceeded, the gateway returns `429 Too Many Requests`.

1. Check if the tenant is on the free tier: `GET /admin/usage` — look at the `tier` field.
2. Review current period usage: the `usage.governed_actions` gauge shows `used`, `limit`, and `pct`.
3. If the quota is legitimately exceeded, the operator needs to upload a license key to unlock unlimited actions.
4. If usage seems anomalous (e.g., automated loops), investigate the agent's activity via `GET /admin/tenants/:tenant_id/audit`.

### Feature Gating Errors (SEV4)

When free-tier users attempt to access gated features, the API returns `403` with `error: "FEATURE_NOT_AVAILABLE"`.

1. This is expected behavior, not an incident — gated features require a paid license.
2. If a paid-tier user receives this error, verify the license is valid: `GET /admin/license`.
3. Check entitlements: `GET /admin/license/entitlements` — confirm the expected feature is `true`.

### Rate Limit Misconfiguration (SEV3)

1. Check `rate_limit_exceeded_total` — labels `window` and `scope` identify the affected rule.
2. Check `rate_limit_checks_total{result="exceeded"}` for confirmation.
3. Review tenant rate limit config: `GET /admin/tenants/:tenant_id`.
4. Update if needed: `PUT /admin/settings/default-rate-limit`.
5. Rate limiting requires `RBTR_FEATURE_RATE_LIMITING=true` to be active.

## Admin Write Lock (Emergency Freeze)

During an active incident, freeze all configuration changes:

```bash
# Enable write lock
curl -X PUT http://localhost:8080/admin/settings/admin-write-lock \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"admin_write_lock": true}'
```

While the write lock is active, all mutating admin endpoints return `403 Forbidden`. Read operations and the public decision API continue to work.

```bash
# Disable write lock after incident resolution
curl -X PUT http://localhost:8080/admin/settings/admin-write-lock \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"admin_write_lock": false}'
```

## Postmortem Template

After resolving a SEV1 or SEV2 incident, complete a postmortem within 48 hours.

```markdown
# Incident Postmortem: [Title]

**Date:** YYYY-MM-DD
**Severity:** SEV1/SEV2
**Duration:** HH:MM start → HH:MM resolved
**Author:** [Name]

## Summary
One-paragraph description of what happened and the impact.

## Timeline
| Time (UTC) | Event |
|------------|-------|
| HH:MM | First alert fired |
| HH:MM | Incident declared |
| HH:MM | Root cause identified |
| HH:MM | Mitigation applied |
| HH:MM | Service fully restored |

## Root Cause
Technical explanation of what caused the incident.

## Impact
- Users affected: [count or scope]
- Decisions blocked/delayed: [count]
- Data loss: [none / description]

## What Went Well
- [Item]

## What Went Poorly
- [Item]

## Action Items
| Action | Owner | Due Date | Status |
|--------|-------|----------|--------|
| [Action] | [Name] | YYYY-MM-DD | Open |
```
