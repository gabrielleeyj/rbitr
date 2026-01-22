-- +goose Up
ALTER TABLE rbitr.approval_requests
    ADD COLUMN IF NOT EXISTS approval_token_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS decided_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS decided_by TEXT NULL,
    ADD COLUMN IF NOT EXISTS decision_comment TEXT NULL,
    ADD COLUMN IF NOT EXISTS executed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS executed_request_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS executed_decision_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS request_decision_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS action_summary TEXT NULL,
    ADD COLUMN IF NOT EXISTS risk TEXT NULL,
    ADD COLUMN IF NOT EXISTS rule_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS reasons JSONB NULL;

CREATE INDEX IF NOT EXISTS idx_approval_requests_tenant_status_time
    ON rbitr.approval_requests (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_approval_requests_tenant_expires
    ON rbitr.approval_requests (tenant_id, expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_approval_requests_tenant_expires;
DROP INDEX IF EXISTS idx_approval_requests_tenant_status_time;

ALTER TABLE rbitr.approval_requests
    DROP COLUMN IF EXISTS approval_token_hash,
    DROP COLUMN IF EXISTS decided_at,
    DROP COLUMN IF EXISTS decided_by,
    DROP COLUMN IF EXISTS decision_comment,
    DROP COLUMN IF EXISTS executed_at,
    DROP COLUMN IF EXISTS executed_request_id,
    DROP COLUMN IF EXISTS executed_decision_id,
    DROP COLUMN IF EXISTS request_decision_id,
    DROP COLUMN IF EXISTS action_summary,
    DROP COLUMN IF EXISTS risk,
    DROP COLUMN IF EXISTS rule_id,
    DROP COLUMN IF EXISTS reasons;
