-- +goose Up
ALTER TABLE rbitr.notification_config
  ADD COLUMN IF NOT EXISTS email_region TEXT NULL,
  ADD COLUMN IF NOT EXISTS email_domain TEXT NULL;

-- +goose Down
ALTER TABLE rbitr.notification_config
  DROP COLUMN IF EXISTS email_region,
  DROP COLUMN IF EXISTS email_domain;
