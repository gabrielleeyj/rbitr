-- +goose Up

CREATE TABLE IF NOT EXISTS rbitr.ticketing_config (
    tenant_id         TEXT PRIMARY KEY REFERENCES rbitr.tenants(tenant_id),
    provider          TEXT NOT NULL DEFAULT '',
    enabled           BOOLEAN NOT NULL DEFAULT FALSE,
    base_url          TEXT NOT NULL DEFAULT '',
    secret_ref        TEXT NOT NULL DEFAULT '',
    project_key       TEXT NOT NULL DEFAULT '',
    issue_type        TEXT NOT NULL DEFAULT '',
    auto_create       BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_secret_ref TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rbitr.ticket_links (
    ticket_link_id      TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL REFERENCES rbitr.tenants(tenant_id),
    approval_request_id TEXT NOT NULL,
    provider            TEXT NOT NULL,
    external_key        TEXT NOT NULL,
    external_url        TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'OPEN',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, approval_request_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_ticket_links_tenant_approval
    ON rbitr.ticket_links (tenant_id, approval_request_id);

-- +goose Down

DROP TABLE IF EXISTS rbitr.ticket_links;
DROP TABLE IF EXISTS rbitr.ticketing_config;
