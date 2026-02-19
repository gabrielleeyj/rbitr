-- +goose Up

ALTER TABLE rbitr.tenants
	ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_tenants_deleted_at
	ON rbitr.tenants (deleted_at);

-- +goose Down

DROP INDEX IF EXISTS rbitr.idx_tenants_deleted_at;

ALTER TABLE rbitr.tenants
	DROP COLUMN IF EXISTS deleted_at;
