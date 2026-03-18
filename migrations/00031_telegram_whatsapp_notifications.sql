-- +goose Up
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS telegram_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS telegram_secret_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS telegram_chat_id TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS whatsapp_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS whatsapp_secret_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS whatsapp_phone_number_id TEXT NOT NULL DEFAULT '';
ALTER TABLE rbitr.notification_config ADD COLUMN IF NOT EXISTS whatsapp_default_recipient TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS telegram_enabled;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS telegram_secret_ref;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS telegram_chat_id;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS whatsapp_enabled;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS whatsapp_secret_ref;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS whatsapp_phone_number_id;
ALTER TABLE rbitr.notification_config DROP COLUMN IF EXISTS whatsapp_default_recipient;
