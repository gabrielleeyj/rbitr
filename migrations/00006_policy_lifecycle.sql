-- +goose Up
CREATE TABLE IF NOT EXISTS rbitr.policy_versions (
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    policy_version TEXT NOT NULL,
    rego_module TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NULL,
    notes TEXT NULL,
    PRIMARY KEY (tenant_id, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_policy_versions_tenant_time
ON rbitr.policy_versions (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS rbitr.tenant_config (
    tenant_id TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id),
    active_policy_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS rbitr.admin_audit_events (
    audit_event_id TEXT PRIMARY KEY,
    tenant_id TEXT NULL,
    actor_type TEXT NOT NULL DEFAULT 'admin_key',
    actor_id TEXT NULL,
    actor_display TEXT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NULL,
    before JSONB NULL,
    after JSONB NULL,
    request_id TEXT NULL,
    ip INET NULL,
    user_agent TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT action_format_chk CHECK (action ~ '^[A-Z0-9]+(\\.[A-Z0-9_]+)+$'),
    CONSTRAINT resource_type_chk CHECK (resource_type ~ '^[A-Z0-9]+(\\.[A-Z0-9_]+)*$')
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_tenant_time
ON rbitr.admin_audit_events (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_time
ON rbitr.admin_audit_events (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_action
ON rbitr.admin_audit_events (action);

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_resource
ON rbitr.admin_audit_events (resource_type, resource_id);

INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, notes)
SELECT tenant_id, policy_version, rego_module, updated_at, 'migrated from rbitr.policies'
FROM rbitr.policies
ON CONFLICT DO NOTHING;

INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at)
SELECT tenant_id, policy_version, updated_at, updated_at
FROM rbitr.policies
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS rbitr.admin_audit_events;
DROP TABLE IF EXISTS rbitr.tenant_config;
DROP TABLE IF EXISTS rbitr.policy_versions;
