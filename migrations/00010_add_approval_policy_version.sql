-- +goose Up
ALTER TABLE rbitr.approval_requests
    ADD COLUMN IF NOT EXISTS policy_version TEXT NULL;

-- +goose Down
ALTER TABLE rbitr.approval_requests
    DROP COLUMN IF EXISTS policy_version;
