-- +goose Up

-- Add enabled flag to tenants (default true for existing tenants)
ALTER TABLE rbitr.tenants ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;

-- Add revoked_at to tenant_keys for key revocation support
ALTER TABLE rbitr.tenant_keys ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ NULL;

-- +goose Down

ALTER TABLE rbitr.tenant_keys DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE rbitr.tenants DROP COLUMN IF EXISTS enabled;
