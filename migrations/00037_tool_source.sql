-- +goose Up
ALTER TABLE rbitr.tools ADD COLUMN source TEXT NOT NULL DEFAULT 'admin';

UPDATE rbitr.tools SET source = 'dev_seed' WHERE tool_id IN ('mock_internal', 'jira');

-- +goose Down
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS source;
