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

        decision_obj(decision, risk, rule_id, priority, code, message) := {
            "version": "2026-01-20",
            "decision": decision,
            "risk": risk,
            "rule": {"id": rule_id, "priority": priority},
            "reasons": [{"code": code, "message": message}],
            "constraints": {}
        }

        default decision := decision_obj("DENY", input.action_risk, "rule_default_deny", 100, "DEFAULT_DENY", "Default deny")

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

        decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
            deny_actions[input.action_type]
        } else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
            require_approval_actions[input.action_type]
        } else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
            allow_actions[input.action_type]
        } else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high risk") if {
            input.action_risk == "HIGH" or input.action_risk == "CRITICAL"
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
