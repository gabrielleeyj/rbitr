-- +goose Up
ALTER TABLE rbitr.policy_versions
    ADD COLUMN IF NOT EXISTS structured_json JSONB NULL;
ALTER TABLE rbitr.policy_versions
    ADD COLUMN IF NOT EXISTS authoring_mode TEXT NOT NULL DEFAULT 'rego';

-- +goose Down
ALTER TABLE rbitr.policy_versions DROP COLUMN IF EXISTS authoring_mode;
ALTER TABLE rbitr.policy_versions DROP COLUMN IF EXISTS structured_json;
