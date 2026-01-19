-- +goose Up
CREATE SCHEMA IF NOT EXISTS rbitr;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS rbitr.tenants (
    tenant_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rbitr.tenant_keys (
    tenant_id TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id),
    key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rbitr.admin_keys (
    admin_key_id TEXT PRIMARY KEY,
    key_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rbitr.tools (
    tool_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    base_url TEXT NOT NULL,
    auth_type TEXT NOT NULL,
    auth_value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tool_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS rbitr.policies (
    policy_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    rego_module TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rbitr.action_decisions (
    decision_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    agent_id TEXT NOT NULL,
    tool_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    action_risk TEXT NOT NULL,
    action_summary TEXT NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_hash TEXT,
    approval_request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rbitr.approval_requests (
    approval_request_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    agent_id TEXT NOT NULL,
    tool_id TEXT NOT NULL,
    action_type TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rbitr.system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO rbitr.tenants (tenant_id, name) VALUES
    ('t_demo', 'Demo Tenant')
ON CONFLICT DO NOTHING;

INSERT INTO rbitr.tenant_keys (tenant_id, key_hash) VALUES
    ('t_demo', encode(digest('tenant_demo_key', 'sha256'), 'hex'))
ON CONFLICT DO NOTHING;

INSERT INTO rbitr.admin_keys (admin_key_id, key_hash, scopes) VALUES
    ('admin_demo', encode(digest('admin_demo_key', 'sha256'), 'hex'), ARRAY['admin:read', 'admin:write'])
ON CONFLICT DO NOTHING;

INSERT INTO rbitr.tools (tool_id, tenant_id, base_url, auth_type, auth_value) VALUES
    ('mock_internal', 't_demo', 'http://localhost:8090', 'api_key', 'mock_internal_key'),
    ('jira', 't_demo', 'http://localhost:8081', 'bearer', 'jira_token')
ON CONFLICT DO NOTHING;

INSERT INTO rbitr.policies (policy_id, tenant_id, rego_module, policy_version, updated_at) VALUES
    (
        'policy_demo',
        't_demo',
        $$
        package rbitr.policy

        default decision = {
            "decision": "DENY",
            "rule_id": "rule_default_deny",
            "reason": "Default deny",
            "policy_version": "p_v1"
        }

        allow_actions := {
            "TICKET.CREATE",
            "TICKET.COMMENT",
            "TICKET.UPDATE",
            "CRM.READ",
            "DATA.READ",
            "DATA.QUERY"
        }

        require_approval_actions := {
            "PAYMENT.REFUND",
            "ACCESS.ROLE_CHANGE"
        }

        deny_actions := {
            "DATA.EXPORT",
            "DATA.BULK_EXPORT",
            "ACCESS.GRANT",
            "DATA.DELETE",
            "CRM.DELETE"
        }

        decision := {
            "decision": "ALLOW",
            "rule_id": "rule_allow_basic_actions_v1",
            "reason": "Policy: allow basic actions",
            "policy_version": "p_v1"
        } {
            allow_actions[input.action_type]
        } else := {
            "decision": "REQUIRE_APPROVAL",
            "rule_id": "rule_require_approval_v1",
            "reason": "Policy: approval required",
            "policy_version": "p_v1"
        } {
            require_approval_actions[input.action_type]
        } else := {
            "decision": "DENY",
            "rule_id": "rule_deny_sensitive_v1",
            "reason": "Policy: deny sensitive action",
            "policy_version": "p_v1"
        } {
            deny_actions[input.action_type]
        }
        $$,
        'p_v1',
        now()
    )
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS rbitr.system_settings;
DROP TABLE IF EXISTS rbitr.approval_requests;
DROP TABLE IF EXISTS rbitr.action_decisions;
DROP TABLE IF EXISTS rbitr.policies;
DROP TABLE IF EXISTS rbitr.tools;
DROP TABLE IF EXISTS rbitr.admin_keys;
DROP TABLE IF EXISTS rbitr.tenant_keys;
DROP TABLE IF EXISTS rbitr.tenants;
DROP SCHEMA IF EXISTS rbitr;
