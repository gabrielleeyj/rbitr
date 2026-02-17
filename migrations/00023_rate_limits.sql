-- +goose Up

ALTER TABLE rbitr.tenant_config
    ADD COLUMN IF NOT EXISTS default_rate_limit_per_minute BIGINT NULL,
    ADD COLUMN IF NOT EXISTS default_rate_limit_per_day BIGINT NULL,
    ADD COLUMN IF NOT EXISTS default_rate_limit_scope TEXT NULL;

ALTER TABLE rbitr.tenant_config
    DROP CONSTRAINT IF EXISTS tenant_config_rate_limit_scope_chk;

ALTER TABLE rbitr.tenant_config
    ADD CONSTRAINT tenant_config_rate_limit_scope_chk
    CHECK (
        default_rate_limit_scope IS NULL OR
        default_rate_limit_scope IN ('tenant', 'tenant_agent', 'tenant_tool', 'tenant_agent_tool')
    );

CREATE TABLE IF NOT EXISTS rbitr.rate_limit_counters (
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    agent_id TEXT NOT NULL DEFAULT '',
    tool_id TEXT NOT NULL DEFAULT '',
    action_type TEXT NOT NULL DEFAULT '',
    window_unit TEXT NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    count BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, agent_id, tool_id, action_type, window_unit, bucket_start),
    CONSTRAINT rate_limit_window_chk CHECK (window_unit IN ('minute', 'day'))
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_counters_updated_at
    ON rbitr.rate_limit_counters (updated_at);

INSERT INTO rbitr.system_settings (key, value, updated_at)
VALUES
    ('default_rate_limit_per_minute', '60', NOW()),
    ('default_rate_limit_per_day', '10000', NOW()),
    ('default_rate_limit_scope', 'tenant_agent_tool', NOW())
ON CONFLICT (key) DO NOTHING;

-- +goose Down

DELETE FROM rbitr.system_settings
WHERE key IN ('default_rate_limit_per_minute', 'default_rate_limit_per_day', 'default_rate_limit_scope');

DROP TABLE IF EXISTS rbitr.rate_limit_counters;

ALTER TABLE rbitr.tenant_config
    DROP CONSTRAINT IF EXISTS tenant_config_rate_limit_scope_chk;

ALTER TABLE rbitr.tenant_config
    DROP COLUMN IF EXISTS default_rate_limit_scope,
    DROP COLUMN IF EXISTS default_rate_limit_per_day,
    DROP COLUMN IF EXISTS default_rate_limit_per_minute;
