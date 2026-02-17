-- +goose Up
ALTER TABLE rbitr.tenant_config
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE rbitr.tenant_config
    DROP COLUMN IF EXISTS version;
