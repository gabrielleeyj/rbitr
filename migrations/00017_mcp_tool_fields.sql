-- +goose Up
-- Add MCP-related fields to tools table for MCP Streamable HTTP transport support

-- Add transport enum (default 'http_api' for existing tools)
ALTER TABLE rbitr.tools
ADD COLUMN transport TEXT NOT NULL DEFAULT 'http_api'
CHECK (transport IN ('http_api', 'mcp_streamable_http'));

-- Add MCP upstream URL (only required when transport = 'mcp_streamable_http')
ALTER TABLE rbitr.tools
ADD COLUMN mcp_upstream_url TEXT;

-- Add description for MCP tool discovery (docstring)
ALTER TABLE rbitr.tools
ADD COLUMN description TEXT;

-- Add input schema for MCP tool discovery (JSON Schema)
ALTER TABLE rbitr.tools
ADD COLUMN input_schema_json JSONB;

-- Add constraint: mcp_upstream_url must be set when transport is 'mcp_streamable_http'
ALTER TABLE rbitr.tools
ADD CONSTRAINT tools_mcp_upstream_url_check
CHECK (
    (transport = 'mcp_streamable_http' AND mcp_upstream_url IS NOT NULL)
    OR
    (transport != 'mcp_streamable_http')
);

-- Update demo tools with descriptions and schemas for testing
UPDATE rbitr.tools
SET
    description = 'Internal demo tool for testing governed actions. Supports refund, charge, cancel, export, and other test operations.',
    input_schema_json = '{
        "type": "object",
        "additionalProperties": true,
        "properties": {
            "action": {
                "type": "string",
                "description": "Action to perform",
                "enum": ["refund", "charge", "cancel", "export", "delete"]
            },
            "amount": {
                "type": "number",
                "description": "Amount for financial operations"
            },
            "customer_id": {
                "type": "string",
                "description": "Customer identifier"
            }
        }
    }'::jsonb
WHERE tool_id = 'mock_internal' AND tenant_id = 't_demo';

UPDATE rbitr.tools
SET
    description = 'Create and manage Jira issues. Use this to create tickets, comment, transition issues, and search.',
    input_schema_json = '{
        "type": "object",
        "additionalProperties": false,
        "properties": {
            "action": {
                "type": "string",
                "enum": ["issue_create", "issue_comment", "issue_transition", "issue_search"]
            },
            "projectKey": {"type": "string"},
            "issueType": {"type": "string"},
            "summary": {"type": "string"},
            "description": {"type": "string"},
            "issueKey": {"type": "string"},
            "comment": {"type": "string"},
            "transitionId": {"type": "string"},
            "jql": {"type": "string"}
        },
        "required": ["action"]
    }'::jsonb
WHERE tool_id = 'jira' AND tenant_id = 't_demo';

-- +goose Down
ALTER TABLE rbitr.tools DROP CONSTRAINT IF EXISTS tools_mcp_upstream_url_check;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS input_schema_json;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS description;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS mcp_upstream_url;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS transport;
