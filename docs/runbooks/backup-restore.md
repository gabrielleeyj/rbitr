# Backup & Restore Runbook

This runbook covers PostgreSQL backup strategies, restore procedures, and audit data considerations for rbitr.

## PostgreSQL Backup Strategy

rbitr uses PostgreSQL as its sole persistent store. Two complementary backup approaches are recommended:

| Method | Purpose | RPO | Complexity |
|--------|---------|-----|------------|
| `pg_dump` logical backup | Full or partial snapshot | Point-in-time when dump ran | Low |
| WAL archiving (continuous) | Point-in-time recovery (PITR) | Seconds (last archived WAL) | Medium |

**Recommended baseline:** daily logical backup + continuous WAL archiving.

## Backup Procedures

### Full Logical Backup

```bash
# Custom-format dump (compressed, supports parallel restore)
pg_dump -Fc -d "$DATABASE_URL" -f rbitr_$(date +%Y%m%d_%H%M%S).dump
```

### Schema-Only Backup

```bash
pg_dump -Fc --schema-only -d "$DATABASE_URL" -f rbitr_schema_$(date +%Y%m%d).dump
```

### Data-Only Backup

```bash
pg_dump -Fc --data-only -d "$DATABASE_URL" -f rbitr_data_$(date +%Y%m%d).dump
```

### Automated Schedule (cron)

```cron
# Daily full backup at 02:00 UTC, retain 30 days
0 2 * * * pg_dump -Fc -d "$DATABASE_URL" -f /backups/rbitr_$(date +\%Y\%m\%d).dump && find /backups -name "rbitr_*.dump" -mtime +30 -delete
```

### WAL Archiving

Configure in `postgresql.conf`:

```ini
wal_level = replica
archive_mode = on
archive_command = 'cp %p /wal_archive/%f'
```

## Restore Procedures

### Full Restore

```bash
# Drop and recreate database
dropdb rbitr
createdb rbitr

# Restore from custom-format dump
pg_restore -d rbitr rbitr_20260304.dump
```

### Point-in-Time Recovery (PITR)

1. Stop PostgreSQL.
2. Replace the data directory with the base backup.
3. Create `recovery.conf` (or `postgresql.auto.conf` on PG 12+):

```ini
restore_command = 'cp /wal_archive/%f %p'
recovery_target_time = '2026-03-04 14:30:00 UTC'
```

4. Start PostgreSQL — it replays WAL up to the target time.

### Single-Table Restore

```bash
# Restore only the tenants table from a full dump
pg_restore -d rbitr --table=tenants rbitr_20260304.dump
```

## Cloud-Managed Backups

### AWS RDS

- **Automated backups:** enabled by default, 1–35 day retention window.
- **Manual snapshots:** `aws rds create-db-snapshot --db-instance-identifier rbitr-prod --db-snapshot-identifier rbitr-manual-20260304`
- **PITR:** restore to any second within the retention window via `aws rds restore-db-instance-to-point-in-time`.

### GCP Cloud SQL

- **Automated backups:** enabled by default, configurable window.
- **On-demand backup:** `gcloud sql backups create --instance=rbitr-prod`
- **PITR:** requires binary logging enabled; restore via `gcloud sql backups restore`.

## Verification

After every restore, validate the database state:

1. **Restore to a staging environment** — never validate on production.
2. **Check row counts** for critical tables:

```sql
SELECT 'tenants' AS tbl, count(*) FROM tenants
UNION ALL SELECT 'admin_keys', count(*) FROM admin_keys
UNION ALL SELECT 'tools', count(*) FROM tools
UNION ALL SELECT 'audit_events', count(*) FROM audit_events
UNION ALL SELECT 'usage_meters', count(*) FROM rbitr.usage_meters
UNION ALL SELECT 'license_history', count(*) FROM rbitr.license_history;
```

3. **Run the readiness check:**

```bash
curl -sf http://localhost:8080/readyz
# Expected: {"status":"ready"}
```

4. **Validate audit chain integrity** — ensure hash chain is unbroken:

```sql
-- Spot-check: verify the latest event's prev_hash matches the prior row
SELECT id, prev_hash FROM audit_events ORDER BY id DESC LIMIT 2;
```

## Audit Data Considerations

### Immutability

The `audit_events` table is protected by the `block_audit_mutations` trigger (see `migrations/00014_audit_hash_chain.sql`). This trigger blocks `UPDATE` and `DELETE` operations on audit rows, ensuring tamper-evidence.

> **Important:** `DELETE` via SQL is blocked by the trigger. Audit data can only be removed by the retention scheduler (which temporarily disables the trigger within an advisory-locked transaction).

### Retention Policy

- The `audit_retention_days` setting controls how long audit events are kept (default: 365 days).
- The retention scheduler (`internal/retention/audit_retention.go`) runs every 24 hours, acquires a PostgreSQL advisory lock, and deletes events older than the cutoff.
- Update retention via the admin API: `PUT /admin/settings/audit-retention` with `{"audit_retention_days": 90}`.
- **Tier-aware retention:** Free-tier installations enforce 7-day audit retention. Paid-tier installations default to 90 days (configurable up to 1 year). The effective retention is resolved from the current license entitlements.

### Recovery Implications

- Audit events deleted by the retention scheduler **cannot be recovered** unless a backup taken before deletion is available.
- When restoring a backup, the restored audit chain will be intact up to the backup timestamp. Events created after the backup are lost.

## Connection Pool Configuration

The gateway connects to PostgreSQL with a configurable connection pool (`internal/db/db.go`):

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DATABASE_URL` | `postgres://postgres@localhost:2345/rbitr?sslmode=require` | Connection string |
| `DB_MAX_OPEN_CONNS` | `30` | Maximum open connections |
| `DB_MAX_IDLE_CONNS` | `10` | Maximum idle connections |
| `DB_CONN_MAX_LIFETIME_SECONDS` | `1800` (30 min) | Connection max lifetime |
| `DB_CONN_MAX_IDLE_TIME_SECONDS` | `300` (5 min) | Idle connection max lifetime |

During restore, ensure the connection pool settings match the target database capacity.
