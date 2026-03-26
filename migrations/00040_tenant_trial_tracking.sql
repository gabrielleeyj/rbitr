-- Add trial_started_at to tenant_config for tracking free tier trial period.
-- +goose Up
ALTER TABLE rbitr.tenant_config
    ADD COLUMN trial_started_at TIMESTAMPTZ NULL;

-- Backfill trial_started_at for existing tenants to their created_at
UPDATE rbitr.tenant_config tc
SET trial_started_at = t.created_at
FROM rbitr.tenants t
WHERE tc.tenant_id = t.tenant_id
  AND tc.trial_started_at IS NULL;

-- +goose Down
ALTER TABLE rbitr.tenant_config
    DROP COLUMN IF EXISTS trial_started_at;
