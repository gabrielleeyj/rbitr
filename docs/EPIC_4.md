# EPIC 4 — Alerting & Notifications

## Purpose

Epic 4 adds a reliable, high-signal **alerting and notification layer** to rbitr so operators are proactively informed about critical governance events (approvals, security abuse, policy failures) without noise.

This epic introduces:

- Slack notifications (primary, shipped first)
- Email notifications (SES + SendGrid/Mailgun, shipped next)
- Alert suppression + deduplication
- Configurable notification routing and recipients
- Secure secret handling (no secrets in DB, audit, or evidence)
- Background scheduler for approval expiry checks

---

## Goals

1. Notify operators **before** governance issues become incidents.
2. Keep alerts **actionable and low-noise**.
3. Preserve rbitr’s security posture (no secret leakage, auditable config).
4. Be deployable in local, AWS, and GCP environments.

---

## In Scope

- Notification engine with provider abstraction
- Slack integration (Webhook + Bot Token)
- Email integration (Amazon SES, SendGrid, Mailgun)
- Deduplication, suppression, and rate limiting
- Admin + UI configuration for notifications
- Mailing lists for email recipients
- Background scheduler for approval expiry alerts
- Metrics and admin audit events

## Out of Scope

- PagerDuty / Opsgenie
- User-level notification preferences
- On-call schedules or escalation chains
- Interactive Slack actions (future)

---

## High-Level Architecture

### High-level flow

- Core code emits a `NotificationEvent` (approval expiring, invalid policy output, etc.)
- Notification engine routes it using tenant settings
- Engine checks suppression/dedup state
- If allowed → deliver via provider (Slack or Email)
- Record delivery outcome metrics + (optional) delivery log row

```
Event (approval, policy, security)
↓
Notification Engine
↓
Dedup / Suppression / Rate Limit
↓
Notifier (Slack / Email)
↓
External Provider
```

### Provider abstraction

```go
type Notifier interface {
  Send(ctx context.Context, msg NotificationMessage) error
  Name() string
}
```

---

## Supported Notification Channels

### Slack (Phase 1)

Two modes are supported:

1. **Incoming Webhook**
   - Simple, fire-and-forget alerts
   - Fixed channel
   - Minimal setup

2. **Bot Token (chat.postMessage)**
   - Channel routing support
   - Richer future extensibility
   - Requires Slack App + Bot Token

Slack is the **primary operational channel**.

### Email (Phase 2)

Supported providers:

- Amazon SES
  - Config: region, from domain, secret ref for AWS creds if needed
- SendGrid
  - Config: API key secret ref
- Mailgun
  - Config: API key + domain secret ref

Email uses **mailing lists** (grouped recipients) instead of per-user config.

### Mailing lists UX

- Create “Security” and “Approvers” lists
- Tenant selects default list
- Event toggles can target specific list later (v2)

---

## Default Alert Behavior

- **Cooldown window:** 10 minutes
- **Approval expiring soon:** 5 minutes before expiry
- Alerts are deduplicated per `(tenant, event_type, resource, severity, channel)`

---

## Event Types (v1)

| Event Type            | Severity | Description                          |
| --------------------- | -------- | ------------------------------------ |
| APPROVAL.EXPIRING     | WARN     | Approval nearing expiry              |
| APPROVAL.EXPIRED      | WARN     | Approval expired without action      |
| SECURITY.TOKEN_ABUSE  | CRITICAL | Repeated invalid token/hash attempts |
| POLICY.INVALID_OUTPUT | CRITICAL | Invalid or malformed policy output   |
| POLICY.EVAL_ERROR     | CRITICAL | Policy evaluation failure            |

---

## Database Schema

### notification_config

```sql
CREATE TABLE IF NOT EXISTS rbitr.notification_config (
  tenant_id                 TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id) ON DELETE CASCADE,

  -- Slack webhook (simple alerts)
  slack_webhook_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
  slack_webhook_secret_ref   TEXT NULL,          -- secret://... (stores webhook URL in secret manager)
  slack_webhook_default_channel TEXT NULL,       -- informational only; webhook decides channel

  -- Slack bot (routing)
  slack_bot_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
  slack_bot_secret_ref       TEXT NULL,          -- secret://... (bot token)
  slack_bot_default_channel  TEXT NULL,          -- channel ID preferred (e.g., C01234567)
  slack_bot_signing_secret_ref TEXT NULL,        -- optional future (interactive, events)

  -- Email (Phase 2)
  email_enabled              BOOLEAN NOT NULL DEFAULT FALSE,
  email_provider             TEXT NULL,          -- 'ses' | 'sendgrid' | 'mailgun'
  email_secret_ref           TEXT NULL,          -- provider API key / creds in secret manager
  email_from                 TEXT NULL,
  email_default_mailing_list_id TEXT NULL,

  -- Routing toggles (minimal set; can expand)
  notify_approval_expiring   BOOLEAN NOT NULL DEFAULT TRUE,
  notify_token_abuse         BOOLEAN NOT NULL DEFAULT TRUE,
  notify_policy_invalid      BOOLEAN NOT NULL DEFAULT TRUE,

  created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_config_updated_at
  ON rbitr.notification_config(updated_at DESC);
```

### mailing_lists

```sql
CREATE TABLE rbitr.mailing_lists (
mailing_list_id TEXT PRIMARY KEY,
tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
name TEXT NOT NULL,
description TEXT NULL,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
UNIQUE (tenant_id, name)
);

CREATE TABLE rbitr.mailing_list_members (
mailing_list_id TEXT NOT NULL REFERENCES rbitr.mailing_lists(mailing_list_id),
email TEXT NOT NULL,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
PRIMARY KEY (mailing_list_id, email)
);

CREATE INDEX IF NOT EXISTS idx_mailing_lists_tenant
  ON rbitr.mailing_lists(tenant_id, created_at DESC);

```

### notification_suppressions

```sql
-- 3) Suppression / dedup state (persistent across restarts)
CREATE TABLE IF NOT EXISTS rbitr.notification_suppressions (
  dedup_key         TEXT PRIMARY KEY,
  tenant_id         TEXT NOT NULL,
  channel           TEXT NOT NULL,    -- 'slack_webhook' | 'slack_bot' | 'email'
  event_type        TEXT NOT NULL,    -- e.g. 'APPROVAL.EXPIRING'
  resource_id       TEXT NULL,        -- e.g. approval_request_id
  severity          TEXT NOT NULL,    -- 'INFO' | 'WARN' | 'CRITICAL'

  first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_sent_at      TIMESTAMPTZ NULL,
  suppressed_until  TIMESTAMPTZ NULL,

  suppressed_count  BIGINT NOT NULL DEFAULT 0,
  last_payload_hash TEXT NULL,        -- optional; helps avoid re-sending identical payloads

  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_tenant_time
  ON rbitr.notification_suppressions(tenant_id, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_suppressed_until
  ON rbitr.notification_suppressions(suppressed_until);
```

### Notes on schema choices

- Everything secret is a \*\_secret_ref pointing to secret manager, never stored in DB.
- You can store slack_webhook_default_channel as informational for UI (“configured for #alerts”), but the webhook itself determines the channel.
- For bot routing, store slack_bot_default_channel as channel ID, not name (names can change).

## Slack Message Templates (v1)

### Approval Expiring (WARN)

- Header: Approval expiring soon
- Fields:
  - Tenant
  - Approval ID
  - Action
  - Risk
  - Expires in (minutes)

- Actions:
  - View approval
  - Open approvals inbox

## Token Abuse Detected (CRITICAL)

- Header: Possible approval token abuse
- Fields:
  - Tenant
  - Tool
  - Action
  - Failure count in window
- Includes sample request IDs
- Actions:
  - View audit
  - View evidence

## Policy Invalid Output (CRITICAL)

- Header: Policy evaluation failure
- Fields:
  - Tenant
  - Policy version
  - Reason
  - Occurrence count
- Actions:
  - View policy
  - Simulate policy

## Secret Reference Resolver

Minimal secret-ref resolver design (works in compose + AWS + GCP)

### Goals

- DB stores only secret_ref strings like: secret://env/RBTR_SLACK_BOT_TOKEN_T_DEMO
- Runtime resolves to actual secret values using pluggable providers
  - Local Dev: `.env` / docker compose secrets
  - Prod: AWS Secrets Manager / GCP Secret Manager / Vault
- Avoid logging secret values
- Allow local dev via environment variables or docker secrets
- Admin/UI never echoes secrets back; only shows "configured / not configured".

### Secret ref format

Support these schemes:

1. `env://VAR_NAME`

- resolves from process env var

2. `file://absolute/path` (optional for docker secrets)

- reads file content

3. `aws-secretsmanager://secret-name#jsonKey` (later)
4. `gcp-secretmanager://projects/.../secrets/.../versions/latest` (later)

For now implement **env + file** and leave interfaces for cloud managers.

```go
type SecretResolver interface {
  Resolve(ctx context.Context, ref string) (string, error)
}

type CompositeResolver struct {
  providers []SecretProvider
}

type SecretProvider interface {
  Match(ref string) bool
  Resolve(ctx context.Context, ref string) (string, error)
}
```

### Providers v1

- EnvProvider: env://
- FileProvider: file://

### Providers v2 (later)

- AWSSecretsManagerProvider
- GCPSecretManagerProvider

### Operational safety rules

- Resolver never logs secret values
- Return errors that redact the secret content:
  - OK: “secret ref not found: env://RBTR_SLACK_BOT_TOKEN”
  - Not OK: printing the value

### Caching

Add a small in-memory cache:

- key: secret_ref
- TTL: 5 minutes
- Allows secret rotation within a short window without restarts

### Compose support

- In docker-compose:
- Mount docker secrets to `/run/secrets/...`

Set slack_bot_secret_ref = `file:///run/secrets/slack_bot_token`
or use env:

`env://RBTR_SLACK_BOT_TOKEN`

### Supported ref formats (v1)

- `env://VAR_NAME`
- `file:///absolute/path`

### Design

```go
type SecretResolver interface {
  Resolve(ctx context.Context, ref string) (string, error)
}
```

- No secret values logged
- Cache resolved secrets for 5 minutes
- DB stores refs only, never values

Future providers:

- AWS Secrets Manager
- GCP Secret Manager
- Hashicorp Vault

## Background Scheduler: Approval Expiry

### Behavior

- Runs every 60 seconds
- Finds approvals expiring within 5 minutes
- Emits APPROVAL.EXPIRING events (dedup-protected)
- Marks approvals expired when past expiry

### Safety

- Uses PostgreSQL advisory lock to ensure single active scheduler:

```pgsql
pg_try_advisory_lock(hash('approval-expiry-scheduler'))
```

## Admin API

### Notification Config

- `GET /admin/tenants/{tenant_id}/notifications`
- `PUT /admin/tenants/{tenant_id}/notifications`

### Secret refs

- `PUT /admin/tenants/{tenant_id}/notifications/slack-secret-ref`
- `PUT /admin/tenants/{tenant_id}/notifications/email-secret-ref`

### Mailing lists

- `GET /admin/tenants/{tenant_id}/mailing-lists`
- `POST /admin/tenants/{tenant_id}/mailing-lists`
- `PUT /admin/tenants/{tenant_id}/mailing-lists/{mailing_list_id}`
- `DELETE /admin/tenants/{tenant_id}/mailing-lists/{mailing_list_id}`

### Test endpoints

- `POST /admin/tenants/{tenant_id}/notifications/test/slack`
- `POST /admin/tenants/{tenant_id}/notifications/test/email`

All config changes emit admin audit events.

### Audit

- Emit admin_audit_events on all config mutations:
  - `TENANT.NOTIFICATIONS.UPDATE`
  - `TENANT.NOTIFICATIONS.SLACK_SECRET_REF.SET`
  - `TENANT.NOTIFICATIONS.EMAIL_SECRET_REF.SET`
  - `TENANT.MAILING_LIST.CREATE/UPDATE/DELETE`

## Metrics

- `notifications_sent_total{channel,event_type,result}`
- `notifications_suppressed_total{channel,event_type}`
- `notifications_latency_ms{channel}`

## Acceptance Criteria

- Slack alerts are delivered and deduplicated correctly
- Email alerts reach configured mailing lists
- No secrets appear in DB, logs, audits, or evidence
- Approval expiry alerts fire once per approval per window
- Token abuse bursts generate a single summarized alert
- All config changes are auditable
