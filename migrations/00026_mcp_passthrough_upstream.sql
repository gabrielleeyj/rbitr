-- +goose Up
ALTER TABLE rbitr.tenant_config
    ADD COLUMN IF NOT EXISTS mcp_passthrough_upstream_tool_id TEXT;

-- +goose Down
ALTER TABLE rbitr.tenant_config
    DROP COLUMN IF EXISTS mcp_passthrough_upstream_tool_id;
