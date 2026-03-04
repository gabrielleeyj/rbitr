# Disaster Recovery Runbook

This runbook covers disaster recovery planning, failover procedures, and recovery validation for rbitr.

## RTO/RPO Targets

Define targets based on your deployment tier:

| Tier | RPO (data loss tolerance) | RTO (time to recover) | Strategy |
|------|--------------------------|----------------------|----------|
| Standard | < 24 hours | < 4 hours | Daily `pg_dump` + redeploy |
| Enhanced | < 1 hour | < 30 minutes | WAL archiving + automated restore |
| High Availability | < 1 minute | < 5 minutes | Cloud-managed HA (RDS Multi-AZ, Cloud SQL HA) + auto-failover |

**Recommended production baseline:** Enhanced tier — RPO < 1 hour (WAL archiving), RTO < 30 minutes (restore + migration + restart).

## Single Points of Failure

| Component | SPOF Risk | Mitigation |
|-----------|-----------|------------|
| PostgreSQL database | **High** — sole persistent store | Replication, managed HA, regular backups |
| Gateway | **Low** — stateless, horizontally scalable | Run multiple instances behind load balancer |
| UI | **Low** — static assets | Serve from CDN or multiple instances |
| Container registry (ghcr.io) | **Low** — external dependency | Cache images locally or mirror to private registry |

The database is the primary concern for DR. All other components can be redeployed from source/registry.

## Database Recovery

### From Logical Backup

See [backup-restore.md](./backup-restore.md) for detailed procedures.

```bash
# Restore from most recent dump
createdb rbitr_recovered
pg_restore -d rbitr_recovered /backups/rbitr_latest.dump
```

### Point-in-Time Recovery (PITR)

For precise recovery using WAL archiving:

1. Restore the base backup.
2. Configure WAL replay with a target timestamp.
3. Start PostgreSQL and let it replay to the target.

See the PITR section in [backup-restore.md](./backup-restore.md).

### Cloud-Managed Failover

**AWS RDS Multi-AZ:**

- Automatic failover to standby replica (typically < 60 seconds).
- DNS endpoint automatically updates — no application changes needed.
- Monitor via RDS events and CloudWatch.

**GCP Cloud SQL HA:**

- Regional instance with automatic failover.
- Failover is transparent to the application via the connection endpoint.
- Monitor via Cloud SQL insights and Cloud Monitoring.

## Application Recovery

The gateway and UI are stateless — recovery is a fresh deployment.

### Recovery Steps

1. **Ensure database is available** (restored or failed over).

2. **Pull container images:**

```bash
docker pull ghcr.io/<org>/rbitr/gateway:<tag>
docker pull ghcr.io/<org>/rbitr/ui:<tag>
```

3. **Run migrations** (idempotent — safe to run even if already applied):

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

4. **Start the gateway:**

```bash
docker compose up -d gateway
# or
kubectl apply -f deployment.yaml
```

5. **Verify readiness:**

```bash
# Readiness check (includes database connectivity)
curl -sf http://localhost:8080/readyz
# Expected: {"status":"ready"}

# Setup status check
curl -sf http://localhost:8080/setup/status
```

## State Recovery Validation

After restoring from backup, validate the recovered state:

### Critical Data Checks

```sql
-- Tenant count
SELECT count(*) AS tenant_count FROM tenants WHERE deleted_at IS NULL;

-- Admin key count
SELECT count(*) AS admin_key_count FROM admin_keys;

-- Active policy versions per tenant
SELECT tenant_id, active_policy_version FROM tenant_configs;

-- Audit event count and latest timestamp
SELECT count(*) AS event_count, max(created_at) AS latest_event FROM audit_events;
```

### Audit Chain Integrity

The audit trail uses a hash chain for tamper detection. Verify chain integrity:

```sql
-- Check for broken chain links (prev_hash mismatches)
WITH ordered AS (
  SELECT id, hash, prev_hash,
         LAG(hash) OVER (ORDER BY id) AS expected_prev_hash
  FROM audit_events
)
SELECT id FROM ordered
WHERE prev_hash IS NOT NULL
  AND prev_hash != expected_prev_hash;
```

An empty result set confirms chain integrity. Any rows returned indicate potential tampering or incomplete restore.

### Smoke Test

Run the marketplace onboarding verification harness as a comprehensive smoke test:

```bash
./scripts/verify_marketplace_onboarding.sh
```

This validates the full setup → configure → operate flow.

## Multi-Region Considerations

### Database Replication

- **Read replicas:** PostgreSQL streaming replication or cloud-managed read replicas can serve read-heavy admin dashboard queries.
- **Write primary:** All writes must go to the primary database. The gateway does not support multi-primary.
- **Cross-region replication lag:** Monitor replication lag; read replicas may serve stale data.

### Gateway Distribution

- The gateway is stateless and can run in multiple regions with a shared database.
- Each region's gateway instances connect to the same primary database (or a regional read replica for read-heavy paths).
- SSE streams (MCP pass-through) are region-local — clients reconnect to the nearest region.

### Container Image Availability

- Primary: `ghcr.io` (GitHub Container Registry).
- Mitigation: mirror images to a private registry (AWS ECR, GCP Artifact Registry) in each deployment region.
- Release binaries are also available as GitHub Release assets as a fallback.

## Communication Checklist

During a disaster recovery event:

- [ ] **Declare incident** — assign incident commander
- [ ] **Internal notification** — notify engineering and operations teams
- [ ] **Customer communication** — notify affected tenants via configured notification channels (Slack, email) or out-of-band
- [ ] **Status page update** — post initial status and estimated recovery time
- [ ] **Progress updates** — update status every 30 minutes during active recovery
- [ ] **Resolution notice** — confirm service restoration and summarize impact
- [ ] **Schedule postmortem** — within 48 hours (see [incident-response.md](./incident-response.md) for template)
