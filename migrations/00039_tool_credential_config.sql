-- +goose Up
ALTER TABLE rbitr.tools ADD COLUMN credential_config JSONB NULL;

-- +goose Down
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS credential_config;
