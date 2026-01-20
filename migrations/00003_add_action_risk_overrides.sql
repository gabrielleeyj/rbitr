-- +goose Up
CREATE TABLE IF NOT EXISTS rbitr.action_risk_overrides (
    tenant_id TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    action_type TEXT NOT NULL,
    action_risk TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, action_type)
);

-- +goose Down
DROP TABLE IF EXISTS rbitr.action_risk_overrides;
