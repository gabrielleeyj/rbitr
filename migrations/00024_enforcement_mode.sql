-- +goose Up

ALTER TABLE rbitr.tenant_config
    ADD COLUMN IF NOT EXISTS enforcement_mode TEXT NOT NULL DEFAULT 'enforce';

ALTER TABLE rbitr.tenant_config
    DROP CONSTRAINT IF EXISTS tenant_config_enforcement_mode_chk;

ALTER TABLE rbitr.tenant_config
    ADD CONSTRAINT tenant_config_enforcement_mode_chk
    CHECK (enforcement_mode IN ('enforce', 'shadow'));

-- +goose Down

ALTER TABLE rbitr.tenant_config
    DROP CONSTRAINT IF EXISTS tenant_config_enforcement_mode_chk;

ALTER TABLE rbitr.tenant_config
    DROP COLUMN IF EXISTS enforcement_mode;
