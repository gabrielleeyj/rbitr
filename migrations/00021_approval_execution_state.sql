-- +goose Up

ALTER TABLE rbitr.approval_requests
    ADD COLUMN IF NOT EXISTS executing_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS execution_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS last_error_code TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_approval_requests_tenant_status_executing
    ON rbitr.approval_requests (tenant_id, status, executing_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_approval_requests_tenant_status_executing;

ALTER TABLE rbitr.approval_requests
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS execution_id,
    DROP COLUMN IF EXISTS failed_at,
    DROP COLUMN IF EXISTS executing_at;
