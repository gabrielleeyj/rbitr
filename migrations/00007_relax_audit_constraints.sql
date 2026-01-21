-- +goose Up
ALTER TABLE rbitr.admin_audit_events
    DROP CONSTRAINT IF EXISTS action_format_chk,
    DROP CONSTRAINT IF EXISTS resource_type_chk;

ALTER TABLE rbitr.admin_audit_events
    ADD CONSTRAINT action_format_chk CHECK (action ~ '^[A-Z0-9_]+(\\.[A-Z0-9_]+)*$'),
    ADD CONSTRAINT resource_type_chk CHECK (resource_type ~ '^[A-Z0-9_]+(\\.[A-Z0-9_]+)*$');

-- +goose Down
ALTER TABLE rbitr.admin_audit_events
    DROP CONSTRAINT IF EXISTS action_format_chk,
    DROP CONSTRAINT IF EXISTS resource_type_chk;

ALTER TABLE rbitr.admin_audit_events
    ADD CONSTRAINT action_format_chk CHECK (action ~ '^[A-Z0-9]+(\\.[A-Z0-9_]+)+$'),
    ADD CONSTRAINT resource_type_chk CHECK (resource_type ~ '^[A-Z0-9]+(\\.[A-Z0-9_]+)*$');
