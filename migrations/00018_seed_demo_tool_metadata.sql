-- +goose Up
-- Update demo tools with descriptions and input schemas for MCP compatibility

UPDATE rbitr.tools
SET
    description = 'Internal mock tool for testing governance workflows. Supports refund, export, and other demo actions.',
    input_schema_json = '{
        "type": "object",
        "additionalProperties": true,
        "properties": {
            "action": {
                "type": "string",
                "description": "The action to perform (e.g., refund, export)"
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
    description = 'Jira integration for issue management. Supports creating tickets, adding comments, and transitioning issues.',
    input_schema_json = '{
        "type": "object",
        "additionalProperties": false,
        "properties": {
            "action": {
                "type": "string",
                "enum": ["issue_create", "issue_comment", "issue_transition", "issue_search"],
                "description": "The Jira action to perform"
            },
            "projectKey": {
                "type": "string",
                "description": "Jira project key (e.g., RBTR)"
            },
            "issueType": {
                "type": "string",
                "description": "Issue type (e.g., Task, Bug, Story)"
            },
            "summary": {
                "type": "string",
                "description": "Issue summary/title"
            },
            "description": {
                "type": "string",
                "description": "Issue description body"
            },
            "issueKey": {
                "type": "string",
                "description": "Existing issue key for comments/transitions"
            },
            "comment": {
                "type": "string",
                "description": "Comment text to add to issue"
            },
            "transitionId": {
                "type": "string",
                "description": "Transition ID to move issue to new status"
            },
            "jql": {
                "type": "string",
                "description": "JQL query for searching issues"
            }
        },
        "required": ["action"]
    }'::jsonb
WHERE tool_id = 'jira' AND tenant_id = 't_demo';

-- +goose Down
-- Revert to empty descriptions and schemas
UPDATE rbitr.tools
SET
    description = NULL,
    input_schema_json = NULL
WHERE tool_id IN ('mock_internal', 'jira') AND tenant_id = 't_demo';
