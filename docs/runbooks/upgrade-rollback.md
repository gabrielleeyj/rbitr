# Upgrade & Rollback Runbook

This runbook covers application upgrade, database migration, and rollback procedures for rbitr.

## Pre-Upgrade Checklist

Before starting any upgrade:

- [ ] **Backup the database** — see [backup-restore.md](./backup-restore.md)
- [ ] **Record current migration version:** `goose -dir migrations postgres "$DATABASE_URL" status`
- [ ] **Record current container image tag:** `docker inspect --format='{{.Config.Image}}' <container>`
- [ ] **Review release notes** for the target version
- [ ] **Enable admin write lock** if the upgrade touches policy or tenant configuration:

```bash
curl -X PUT http://localhost:8080/admin/settings/admin-write-lock \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"admin_write_lock": true}'
```

## Database Migration

rbitr uses [Goose](https://github.com/pressly/goose) for database migrations. Migrations are in the `migrations/` directory (34 migrations as of this writing, `00001_init.sql` through `00034_usage_meters.sql`).

> **Note:** Migrations `00033` and `00034` introduce the `license_history` and `usage_meters` tables for the freemium tier system. These are additive schema changes with no impact on existing data.

### Run Migrations

**Via docker-compose (recommended):**

The `migrate` service in `docker-compose.yml` runs `goose up` automatically:

```bash
docker compose up migrate
```

**Directly:**

```bash
go install github.com/pressly/goose/v3/cmd/goose@v3.25.0
goose -dir migrations postgres "$DATABASE_URL" up
```

### Verify Migration Status

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

This shows all migrations and whether each has been applied.

## Application Upgrade

### Container Deployment

1. Pull the new image:

```bash
docker pull ghcr.io/<org>/rbitr/gateway:<new-tag>
```

2. Update the image reference in `docker-compose.yml` or your Kubernetes deployment manifest.

3. Run migrations first (if schema changes are included):

```bash
docker compose up migrate
```

4. Rolling restart the gateway:

```bash
# docker-compose
docker compose up -d gateway

# Kubernetes
kubectl rollout restart deployment/rbitr-gateway
```

5. Verify health:

```bash
curl -sf http://localhost:8080/healthz  # Liveness
curl -sf http://localhost:8080/readyz   # Readiness (includes DB check)
```

### Release Pipeline

The release workflow (`.github/workflows/release.yml`) produces:

- Multi-platform binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64)
- SHA256 checksums
- Multi-arch container images pushed to `ghcr.io` (gateway + mocktool)
- Marketplace onboarding report artifact

Quality gates run before release: lint, tests, setup smoke, and marketplace onboarding harness.

## Migration Rollback

Each migration has an explicit `+goose Down` directive for reversibility.

### Revert Last Migration

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

### Revert to a Specific Version

```bash
# Revert one migration at a time until reaching the target
goose -dir migrations postgres "$DATABASE_URL" down-to <version>
```

### Verify After Rollback

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

Confirm the schema matches the expected state by checking table structure for the reverted migration.

## Application Rollback

1. **Revert the container image** to the previous tag:

```bash
# docker-compose: update image tag in docker-compose.yml, then:
docker compose up -d gateway

# Kubernetes:
kubectl set image deployment/rbitr-gateway gateway=ghcr.io/<org>/rbitr/gateway:<old-tag>
```

2. **If a migration was applied**, revert it to match the old application version:

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

> **Important:** The application and database schema must be compatible. If you roll back the application, you must also roll back any migrations that the old version does not understand.

3. **Verify health:**

```bash
curl -sf http://localhost:8080/healthz
curl -sf http://localhost:8080/readyz
```

## Policy Rollback

Policy changes can be rolled back independently of application deployments:

```bash
curl -X PUT http://localhost:8080/admin/tenants/:tenant_id/policies/rollback \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"policy_version": "<previous-version>"}'
```

- Policy rollback does **not** require an application restart.
- View available versions: `GET /admin/tenants/:tenant_id/policies`.
- Simulate before publishing: `POST /admin/tenants/:tenant_id/policies/simulate`.

## Zero-Downtime Upgrade Notes

The rbitr gateway is designed for safe rolling restarts:

- **Stateless gateway:** No in-memory session state. Each request is independently authenticated and evaluated.
- **MCP SSE streams:** Active SSE connections will be dropped during restart. Clients should reconnect. The maximum SSE stream duration is bounded by the request timeout (15 seconds).
- **Rolling restarts:** Safe in both docker-compose and Kubernetes. Kubernetes readiness probe (`/readyz`) ensures traffic only routes to healthy instances.
- **Migration compatibility:** Database migrations should be backwards-compatible for one version — the old application version should continue to work with the new schema during the rolling restart window.

### Recommended Upgrade Sequence

1. Apply database migration (new schema is backwards-compatible).
2. Rolling restart with new application image.
3. Verify all instances are healthy.
4. Disable admin write lock if it was enabled.

```bash
curl -X PUT http://localhost:8080/admin/settings/admin-write-lock \
  -H "Authorization: Bearer <admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"admin_write_lock": false}'
```
