-- Fix usage_meters.tenant_id type: UUID -> TEXT to match rbitr.tenants.tenant_id.
-- +goose Up
-- Drop the existing constraint before altering the column type
ALTER TABLE rbitr.usage_meters
    DROP CONSTRAINT IF EXISTS usage_meters_tenant_id_fkey;

ALTER TABLE rbitr.usage_meters
    ALTER COLUMN tenant_id TYPE TEXT USING tenant_id::TEXT;

-- Re-add the constraint
ALTER TABLE rbitr.usage_meters
    ADD CONSTRAINT usage_meters_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES rbitr.tenants(tenant_id);

-- +goose Down
ALTER TABLE rbitr.usage_meters
    DROP CONSTRAINT IF EXISTS usage_meters_tenant_id_fkey;

ALTER TABLE rbitr.usage_meters
    ALTER COLUMN tenant_id TYPE UUID USING tenant_id::UUID;
