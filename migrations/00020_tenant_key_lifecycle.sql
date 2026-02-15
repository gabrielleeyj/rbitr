-- +goose Up

-- Restructure tenant_keys to support multiple keys per tenant
-- Step 1: Add new columns
ALTER TABLE rbitr.tenant_keys ADD COLUMN IF NOT EXISTS key_id UUID;
ALTER TABLE rbitr.tenant_keys ADD COLUMN IF NOT EXISTS key_prefix TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.tenant_keys ADD COLUMN IF NOT EXISTS rotated_at TIMESTAMPTZ NULL;

-- Step 2: Populate key_id for existing rows
UPDATE rbitr.tenant_keys SET key_id = gen_random_uuid() WHERE key_id IS NULL;

-- Step 3: Drop the old primary key (tenant_id was PK, limiting to one key per tenant)
ALTER TABLE rbitr.tenant_keys DROP CONSTRAINT IF EXISTS tenant_keys_pkey;

-- Step 4: Make key_id the primary key
ALTER TABLE rbitr.tenant_keys ALTER COLUMN key_id SET NOT NULL;
ALTER TABLE rbitr.tenant_keys ADD PRIMARY KEY (key_id);

-- Step 5: Add unique constraint on key_hash for fast lookup
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_keys_key_hash ON rbitr.tenant_keys (key_hash);

-- Step 6: Add index for listing keys by tenant
CREATE INDEX IF NOT EXISTS idx_tenant_keys_tenant_id ON rbitr.tenant_keys (tenant_id);

-- +goose Down

DROP INDEX IF EXISTS rbitr.idx_tenant_keys_tenant_id;
DROP INDEX IF EXISTS rbitr.idx_tenant_keys_key_hash;

-- Restore original schema (best-effort)
ALTER TABLE rbitr.tenant_keys DROP CONSTRAINT IF EXISTS tenant_keys_pkey;
ALTER TABLE rbitr.tenant_keys DROP COLUMN IF EXISTS rotated_at;
ALTER TABLE rbitr.tenant_keys DROP COLUMN IF EXISTS key_prefix;
ALTER TABLE rbitr.tenant_keys DROP COLUMN IF EXISTS key_id;
ALTER TABLE rbitr.tenant_keys ADD PRIMARY KEY (tenant_id);
