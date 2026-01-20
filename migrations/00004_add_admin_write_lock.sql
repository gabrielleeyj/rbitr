-- +goose Up
INSERT INTO rbitr.system_settings (key, value, updated_at)
VALUES ('admin_write_lock', 'false', now())
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM rbitr.system_settings WHERE key = 'admin_write_lock';
