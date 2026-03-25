-- +goose Up
ALTER TABLE rbitr.tools ADD COLUMN openapi_spec_url TEXT NULL;
ALTER TABLE rbitr.tools ADD COLUMN openapi_operation_id TEXT NULL;

-- +goose Down
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS openapi_operation_id;
ALTER TABLE rbitr.tools DROP COLUMN IF EXISTS openapi_spec_url;
