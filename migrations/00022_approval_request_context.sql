-- +goose Up

ALTER TABLE rbitr.approval_requests
    ADD COLUMN IF NOT EXISTS request_context JSONB NULL;

-- +goose Down

ALTER TABLE rbitr.approval_requests
    DROP COLUMN IF EXISTS request_context;
