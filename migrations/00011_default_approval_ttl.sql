-- +goose Up
INSERT INTO rbitr.system_settings (key, value, updated_at)
VALUES ('default_approval_ttl_seconds', '900', NOW())
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM rbitr.system_settings WHERE key = 'default_approval_ttl_seconds';
