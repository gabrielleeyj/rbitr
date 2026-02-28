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
