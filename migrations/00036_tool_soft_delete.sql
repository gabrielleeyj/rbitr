-- Add soft-delete support for tools (archived_at column).
-- +goose Up
ALTER TABLE rbitr.tools ADD COLUMN archived_at TIMESTAMPTZ NULL;

CREATE INDEX idx_tools_active ON rbitr.tools (tenant_id)
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS rbitr.idx_tools_active;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS archived_at;
