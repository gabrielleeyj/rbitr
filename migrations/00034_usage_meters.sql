-- Usage meters track monthly governed action counts per tenant.
-- Used to enforce free-tier quota (10k actions/month).
-- +goose Up
CREATE TABLE IF NOT EXISTS rbitr.usage_meters (
    tenant_id    UUID NOT NULL,
    period       TEXT NOT NULL CHECK (period ~ '^\d{4}-\d{2}$'),
    action_count BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, period)
);

-- Index for efficient period lookups across all tenants (usage dashboard).
CREATE INDEX IF NOT EXISTS idx_usage_meters_period
    ON rbitr.usage_meters(period);

-- +goose Down

DROP TABLE IF EXISTS rbitr.usage_meters;
