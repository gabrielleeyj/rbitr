-- +goose Up
-- Remove legacy demo defaults from earlier bootstrap behavior.
DELETE FROM rbitr.action_decisions WHERE tenant_id = 't_demo';
DELETE FROM rbitr.approval_requests WHERE tenant_id = 't_demo';
DELETE FROM rbitr.tools WHERE tenant_id = 't_demo';
DELETE FROM rbitr.policies WHERE tenant_id = 't_demo';
DELETE FROM rbitr.policy_versions WHERE tenant_id = 't_demo';
DELETE FROM rbitr.tenant_config WHERE tenant_id = 't_demo';
DELETE FROM rbitr.tenant_keys
WHERE tenant_id = 't_demo'
   OR key_hash = encode(digest('tenant_demo_key', 'sha256'), 'hex');
DELETE FROM rbitr.admin_keys
WHERE admin_key_id = 'admin_demo'
   OR key_hash = encode(digest('admin_demo_key', 'sha256'), 'hex');
DELETE FROM rbitr.tenants WHERE tenant_id = 't_demo';

-- +goose Down
-- No-op: do not recreate insecure demo defaults.
SELECT 1;
