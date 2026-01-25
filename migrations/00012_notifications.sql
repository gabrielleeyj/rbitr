-- +goose Up
CREATE TABLE IF NOT EXISTS rbitr.notification_config (
  tenant_id                     TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id) ON DELETE CASCADE,
  slack_webhook_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
  slack_webhook_secret_ref       TEXT NULL,
  slack_webhook_default_channel  TEXT NULL,
  slack_bot_enabled              BOOLEAN NOT NULL DEFAULT FALSE,
  slack_bot_secret_ref           TEXT NULL,
  slack_bot_default_channel      TEXT NULL,
  slack_bot_signing_secret_ref   TEXT NULL,
  email_enabled                  BOOLEAN NOT NULL DEFAULT FALSE,
  email_provider                 TEXT NULL,
  email_secret_ref               TEXT NULL,
  email_from                     TEXT NULL,
  email_default_mailing_list_id  TEXT NULL,
  notify_approval_expiring       BOOLEAN NOT NULL DEFAULT TRUE,
  notify_token_abuse             BOOLEAN NOT NULL DEFAULT TRUE,
  notify_policy_invalid          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_config_updated_at
  ON rbitr.notification_config(updated_at DESC);

CREATE TABLE IF NOT EXISTS rbitr.mailing_lists (
  mailing_list_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS rbitr.mailing_list_members (
  mailing_list_id TEXT NOT NULL REFERENCES rbitr.mailing_lists(mailing_list_id) ON DELETE CASCADE,
  email TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (mailing_list_id, email)
);

CREATE INDEX IF NOT EXISTS idx_mailing_lists_tenant
  ON rbitr.mailing_lists(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS rbitr.notification_suppressions (
  dedup_key         TEXT PRIMARY KEY,
  tenant_id         TEXT NOT NULL,
  channel           TEXT NOT NULL,
  event_type        TEXT NOT NULL,
  resource_id       TEXT NULL,
  severity          TEXT NOT NULL,
  first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_sent_at      TIMESTAMPTZ NULL,
  suppressed_until  TIMESTAMPTZ NULL,
  suppressed_count  BIGINT NOT NULL DEFAULT 0,
  last_payload_hash TEXT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_tenant_time
  ON rbitr.notification_suppressions(tenant_id, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_suppressed_until
  ON rbitr.notification_suppressions(suppressed_until);

-- +goose Down
DROP TABLE IF EXISTS rbitr.notification_suppressions;
DROP TABLE IF EXISTS rbitr.mailing_list_members;
DROP TABLE IF EXISTS rbitr.mailing_lists;
DROP TABLE IF EXISTS rbitr.notification_config;
