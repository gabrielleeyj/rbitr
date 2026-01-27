-- +goose Up
ALTER TABLE rbitr.admin_audit_events ADD COLUMN IF NOT EXISTS stream_id TEXT;
ALTER TABLE rbitr.admin_audit_events ADD COLUMN IF NOT EXISTS event_hash TEXT;
ALTER TABLE rbitr.admin_audit_events ADD COLUMN IF NOT EXISTS prev_hash TEXT;

UPDATE rbitr.admin_audit_events
SET stream_id = COALESCE(tenant_id, 'global')
WHERE stream_id IS NULL;

ALTER TABLE rbitr.admin_audit_events ALTER COLUMN stream_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_admin_audit_events_stream_time
ON rbitr.admin_audit_events (stream_id, created_at DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rbitr.block_audit_mutations()
RETURNS trigger AS $fn$
BEGIN
  RAISE EXCEPTION 'audit events are immutable';
END;
$fn$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS audit_events_immutable ON rbitr.admin_audit_events;
CREATE TRIGGER audit_events_immutable
BEFORE UPDATE OR DELETE ON rbitr.admin_audit_events
FOR EACH ROW EXECUTE FUNCTION rbitr.block_audit_mutations();

INSERT INTO rbitr.system_settings (key, value, updated_at)
VALUES ('audit_retention_days', '365', NOW())
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_immutable ON rbitr.admin_audit_events;
DROP FUNCTION IF EXISTS rbitr.block_audit_mutations;
DROP INDEX IF EXISTS idx_admin_audit_events_stream_time;
ALTER TABLE rbitr.admin_audit_events DROP COLUMN IF EXISTS prev_hash;
ALTER TABLE rbitr.admin_audit_events DROP COLUMN IF EXISTS event_hash;
ALTER TABLE rbitr.admin_audit_events DROP COLUMN IF EXISTS stream_id;
DELETE FROM rbitr.system_settings WHERE key = 'audit_retention_days';
