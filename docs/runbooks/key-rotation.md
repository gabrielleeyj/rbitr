# Key Rotation Runbook

This runbook covers rotation procedures for all secret material in rbitr: tenant keys, admin keys, setup tokens, and notification secrets.

## Tenant Key Rotation

Tenant keys authenticate API consumers against the gateway. The key lifecycle API supports zero-downtime rotation.

### Rotate All Keys (Revoke + Issue New)

```bash
# Revokes all active keys for the tenant and issues a new one
curl -X POST http://localhost:8080/admin/tenants/:tenant_id/keys/rotate \
  -H "Authorization: Bearer <admin-key>"
```

Response includes the new plaintext key. Store it securely — it is not retrievable after this response.

> **Warning:** All existing keys become invalid immediately. Consumers using old keys will receive `401 Unauthorized`.

### Add Key Without Revoking (Zero-Downtime Overlap)

For zero-downtime rotation, use the add-then-revoke pattern:

1. **Add a new key** (old keys remain valid):

```bash
curl -X POST http://localhost:8080/admin/tenants/:tenant_id/keys \
  -H "Authorization: Bearer <admin-key>"
```

2. **Distribute the new key** to all consuming services.
3. **Verify** the new key is in use (monitor `tenant_auth_fallback_total` and `tenant_key_legacy_upgrade_total` — both should be zero for this tenant).
4. **Revoke the old key:**

```bash
curl -X POST http://localhost:8080/admin/tenants/:tenant_id/keys/:key_id/revoke \
  -H "Authorization: Bearer <admin-key>"
```

### List Active Keys

```bash
curl http://localhost:8080/admin/tenants/:tenant_id/keys \
  -H "Authorization: Bearer <admin-key>"
```

Returns key IDs and metadata (not plaintext values). Use this to identify which `key_id` to revoke.

## Admin Key Rotation

Admin keys authenticate operators accessing the admin API. Admin keys have scope-based access control.

There is currently no hot-rotation endpoint for admin keys. Rotation procedure:

1. **Create a new admin key** via the setup flow or direct database insert:

```sql
-- Direct DB insert (use only if setup flow is not available)
INSERT INTO admin_keys (admin_key_id, key_hash, scopes, created_at)
VALUES (gen_random_uuid()::text, encode(digest('<new-key>', 'sha256'), 'hex'),
        ARRAY['tenants:read','tenants:write','tools:read','tools:write','policies:read','policies:write','settings:read','settings:write'],
        now());
```

2. **Verify** the new key works:

```bash
curl http://localhost:8080/admin/me \
  -H "Authorization: Bearer <new-key>"
# Expected: {"admin_key_id":"...","scopes":["..."]}
```

3. **Update all consuming services** (UI, automation, CI) with the new key.
4. **Revoke the old key** by deleting it from the database:

```sql
DELETE FROM admin_keys WHERE admin_key_id = '<old-key-id>';
```

### Scope Reference

Admin keys support granular scopes (see `internal/api/admin/scopes.go`):

| Scope | Access |
|-------|--------|
| `tenants:read` | List/view tenants, keys, config |
| `tenants:write` | Create/update/delete tenants, rotate keys |
| `tools:read` | List/view tools |
| `tools:write` | Update tool config and metadata |
| `policies:read` | List/view policies, simulate |
| `policies:write` | Create/publish/rollback policies, risk overrides |
| `settings:read` | View global settings |
| `settings:write` | Update global settings, write lock |

## Setup Token Rotation

The setup token is used only during initial bootstrapping (`POST /setup/initialize`). It is a one-time-use credential.

### Procedure

1. Generate a new token:

```bash
openssl rand -hex 32
```

2. Update the environment variable:

```bash
# Direct value
export RBTR_SETUP_TOKEN="<new-token>"

# Or file-based
echo -n "<new-token>" > /secrets/setup-token
export RBTR_SETUP_TOKEN_FILE="/secrets/setup-token"
```

3. Restart the gateway to pick up the new value.

> **Note:** The setup token is only required when `RBTR_SETUP_TOKEN_REQUIRED=true`. After initial setup is complete, the setup endpoint is idempotent — re-running it with a valid token is a no-op.

## Notification Secret Rotation

Notification secrets (Slack webhook URLs, email SMTP passwords) are stored as secret references resolved at runtime via `env://` or `file://` URI schemes.

### Update Slack Secret Reference

```bash
curl -X PUT http://localhost:8080/admin/tenants/:tenant_id/notifications/slack-secret-ref \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"slack_secret_ref": "env://RBTR_SECRET_SLACK_WEBHOOK"}'
```

Then update the environment variable:

```bash
export RBTR_SECRET_SLACK_WEBHOOK="https://hooks.slack.com/services/NEW/WEBHOOK/URL"
```

### Update Email Secret Reference

```bash
curl -X PUT http://localhost:8080/admin/tenants/:tenant_id/notifications/email-secret-ref \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"email_secret_ref": "file:///secrets/smtp-password"}'
```

Then update the secret file:

```bash
echo -n "new-smtp-password" > /secrets/smtp-password
```

### Secret Reference Schemes

| Scheme | Example | Resolution |
|--------|---------|------------|
| `env://` | `env://RBTR_SECRET_SLACK_WEBHOOK` | Reads `os.Getenv("RBTR_SECRET_SLACK_WEBHOOK")` |
| `file://` | `file:///secrets/smtp-password` | Reads file contents from the path |

Resolved values are cached with a 5-minute TTL (configurable in `cmd/gateway/main.go`). After updating the underlying secret, allow up to 5 minutes for the new value to take effect, or restart the gateway for immediate refresh.

### Test After Rotation

```bash
# Test Slack delivery
curl -X POST http://localhost:8080/admin/tenants/:tenant_id/notifications/test/slack \
  -H "Authorization: Bearer <admin-key>"

# Test email delivery
curl -X POST http://localhost:8080/admin/tenants/:tenant_id/notifications/test/email \
  -H "Authorization: Bearer <admin-key>"
```

## Verification After Any Key Rotation

1. **Confirm old key is rejected:**

```bash
curl -w "%{http_code}" -o /dev/null -s http://localhost:8080/admin/me \
  -H "Authorization: Bearer <old-key>"
# Expected: 401
```

2. **Confirm new key is accepted:**

```bash
curl -w "%{http_code}" -o /dev/null -s http://localhost:8080/admin/me \
  -H "Authorization: Bearer <new-key>"
# Expected: 200
```

3. **Monitor for legacy key usage:**

| Metric | Meaning |
|--------|---------|
| `tenant_auth_fallback_total` | Requests using deprecated `X-Tenant-Key` header (should be zero) |
| `tenant_key_legacy_upgrade_total` | Keys with legacy SHA-256 hash being upgraded to HMAC-SHA256 |

If either metric is non-zero after rotation, consumers are still using old authentication patterns. Investigate and update them.
