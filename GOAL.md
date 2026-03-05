# GTM Deployment Readiness Goal

This file tracks **current blockers** preventing rbitr from being GTM deployment-ready for:

- Direct production deployment (container service)
- AWS Marketplace listing (Phase 1 — current focus)
- GCP Marketplace listing (Phase 2 — deferred until AWS launch is complete)

Date updated: 2026-03-05

## Current Status Snapshot (2026-03-05)

Status legend:

- `Closed`: implemented and verified in repo/workflows
- `Partial`: meaningful progress landed, but follow-up still required to fully close
- `Open`: not yet implemented to GTM-ready depth

| ID | Priority | Status | Blocker | Current status |
| --- | --- | --- | --- | --- |
| 1 | P0 | Closed | Remove seeded static credentials/demo tenant defaults from production migration path | Demo seed inserts are removed from init path; cleanup migration removes legacy demo rows/hashes. |
| 2 | P0 | Closed | Replace demo/localhost tool defaults with production-safe bootstrap flow | Setup-driven bootstrap is production path; localhost tool defaults are dev-only and explicit via `RBTR_DEV_AUTO_TOOLS`. |
| 3 | P0 | Closed | Make test/build pipeline reproducible and pinned | Test helper tooling in `scripts/test.sh` is version pinned. |
| 4 | P0 | Closed | Resolve toolchain/version drift baseline in build/test path | Go version pinned in `go.mod`, CI setup, and container build images (`1.25.6`). |
| 5 | P0 | Closed | Provide AWS production deployment package artifacts | Helm chart for EKS at `deploy/helm/rbitr/`; CloudFormation ECS Fargate template at `deploy/cloudformation/rbitr-ecs.yaml`; UI image added to release pipeline. |
| 6 | P0 | Closed | Add marketplace-compliant integration/verification harness | Install/upgrade onboarding harness + CI artifact workflow are implemented. |
| 7 | P0 | Open | Implement AWS Marketplace entitlement/metering integration | No AWS Marketplace Metering API (`RegisterUsage`/`MeterUsage`) integration yet. |
| 16 | P2 | Deferred | Implement GCP Marketplace procurement/entitlement integration | Deferred to Phase 2 after AWS launch. |
| 8 | P0 | Closed | Add security policy and vulnerability disclosure docs | Root `SECURITY.md` is implemented with disclosure process and security posture. |
| 9 | P0 | Closed | Add software license/distribution metadata | Root `LICENSE` is implemented with Apache License 2.0 text. |
| 11 | P1 | Closed | Approval execution idempotency/in-flight ownership hardening | Execution claim/state transitions, retry-window idempotency guards, and ADR dedup are implemented and tested. |
| 12 | P1 | Closed | Add full operations runbooks | Operations runbooks published at `docs/runbooks/` covering backup/restore, incident response, key rotation, upgrade/rollback, and disaster recovery. |
| 13 | P1 | Closed | Add standardized API contract artifacts | OpenAPI 3.1 spec published at `docs/openapi.yaml`. |
| 14 | P2 | Deferred | Add browser-level UI e2e smoke tests in CI release gates | API/setup smoke + marketplace onboarding harness are in CI; browser-level UI automation deferred to post-launch. |
| 15 | P1 | Closed | Add explicit production ingress/TLS/certificate guidance | Production ingress/TLS guide published at `docs/production-ingress.md`. |

### Overall Progress (Phase 1 — AWS Marketplace)

- Closed: 12 blockers (`1,2,3,4,5,6,8,9,11,12,13,15`)
- Open: 1 blocker (`7`)
- Deferred to post-launch: 2 blockers (`14` — browser E2E, `16` — GCP Marketplace)

## P0 Blockers (Must Fix Before Any External Production GTM)

1. Remove seeded static credentials and demo tenant defaults from production migration path.
   - Current status: Closed. `migrations/00001_init.sql` no longer seeds demo credentials/tenants.
   - Current status: Closed. `migrations/00028_remove_demo_seed_defaults.sql` removes legacy demo rows/hashes.
   - Blocker reason: insecure bootstrap pattern; fails production security posture.

2. Replace demo/localhost tool defaults with production-safe bootstrap flow.
   - Current status: Closed for production path. Setup bootstrap is the production path.
   - Current status: Closed for production path. localhost wiring remains dev-only and explicit (`RBTR_DEV_AUTO_TOOLS`).
   - Blocker reason: deployment is not immediately usable in real environments without unsafe/manual rewiring.

3. Make test/build pipeline reproducible and pinned.
   - Current status: Closed. `scripts/test.sh` uses pinned helper versions (`v1.13.0`, `v1.2.0`).
   - Blocker reason: non-deterministic CI/release verification; cannot support repeatable GTM releases.

4. Resolve toolchain/version drift baseline in build/test path.
   - Current status: Closed. Toolchain is pinned to Go `1.25.6` in `go.mod`, CI setup, and Dockerfiles.
   - Blocker reason: no trustworthy release-quality pass/fail baseline until deterministic build toolchain is enforced.

## P0 Marketplace Packaging Blockers (AWS — Phase 1)

5. Provide production deployment package artifacts (AWS).
   - Current status: Closed. Helm chart at `deploy/helm/rbitr/` (EKS with optional bundled PostgreSQL, ALB ingress, migration hook, HPA, IRSA). CloudFormation ECS Fargate template at `deploy/cloudformation/rbitr-ecs.yaml` (VPC + RDS + ALB + Secrets Manager). UI image added to release pipeline. Helm lint CI validates chart on PRs.
   - Blocker reason: AWS Marketplace container listing requires standardized deploy artifacts (Helm chart for EKS or ECS task definition + CloudFormation).

6. Add marketplace-compliant integration/verification harness.
   - Current status: Closed. `scripts/verify_marketplace_onboarding.sh` validates fresh install/setup/upgrade flow.
   - Current status: Closed. CI workflow uploads machine-readable report artifact (`.github/workflows/marketplace-onboarding.yml`).
   - Blocker reason: listing review requires reliable install and runtime verification.

7. Implement AWS Marketplace entitlement/metering integration.
   - Current state: no AWS Marketplace Metering API integration (`RegisterUsage` / `MeterUsage`).
   - Blocker reason: paid AWS Marketplace listing cannot launch without entitlement verification and billing wiring.

## Deferred — GCP Marketplace (Phase 2)

16. Implement GCP Marketplace procurement/entitlement integration.
    - Current state: not started. Deferred until AWS Marketplace launch is complete.
    - Requires: Cloud Commerce Partner Procurement API for entitlement, Service Control API for usage reporting.
    - Note: Helm chart from blocker #5 largely carries over; billing/entitlement code is marketplace-specific.

## P0 Security/Compliance Blockers

8. Add security policy and vulnerability disclosure process docs.
   - Current status: Closed. Root `SECURITY.md` now defines reporting process, support policy, and current security posture.
   - Blocker reason: enterprise buyer and marketplace review expectations not met.

9. Add software license/distribution metadata.
   - Current status: Closed. Root `LICENSE` now includes Apache License 2.0.
   - Blocker reason: legal/commercial packaging incomplete for GTM.

## P1 Blockers (Should Fix Before Broad GTM Scale, But Not Always Day-1 Hard Stop)

11. Complete approval execution idempotency/in-flight ownership hardening.
   - Current status: Closed. Fixed `decisionExecuting` typo that caused dead EXECUTING branch code. Added `executed_at` guard in retry paths (both REST and MCP handlers) to prevent re-execution when tool already succeeded. Added `AND executed_at IS NULL` to `MarkApprovalExecuted` SQL to prevent double-mark races. Added ADR dedup check on `approval_request_id` to prevent duplicate audit records on retry.
   - Risk: correctness and financial/control side effects under race/retry conditions.

12. Add full runbooks for operations.
   - Current status: Closed. Operations runbooks published at `docs/runbooks/` covering backup/restore, incident response, key rotation, upgrade/rollback, and disaster recovery.
   - Risk: production support model not complete for enterprise onboarding.

13. Add standardized API contract artifacts (OpenAPI/JSON schemas for external consumers).
   - Current status: Closed. OpenAPI 3.1 specification published at `docs/openapi.yaml` covering all endpoints (health, setup, public v1, admin).
   - Risk: integration friction and higher support burden.

14. Add browser-level UI e2e smoke tests in CI for release gates.
   - Current state: Deferred to post-launch. API/setup smoke + marketplace onboarding harness are already in CI/release gates.
   - Risk: regression escape risk at release time (mitigated by existing API-level smoke tests).

15. Add explicit production ingress/TLS/certificate guidance and reference configs.
   - Current status: Closed. Production deployment guide published at `docs/production-ingress.md` covering Nginx TLS, Kubernetes ingress (nginx-ingress + Traefik), AWS ALB, GCP HTTPS LB, cert-manager, and environment variable reference.
   - Risk: inconsistent or insecure customer deployments.

## Recently Closed (2026-03-05)

- Helm chart for EKS at `deploy/helm/rbitr/` — gateway + UI deployments with probes, ALB ingress with path-based routing, migration job hook (all 29 SQL files in ConfigMap), optional Bitnami PostgreSQL subchart, HPA, IRSA-ready ServiceAccount, `values-production.yaml` with HA defaults.
- CloudFormation ECS Fargate template at `deploy/cloudformation/rbitr-ecs.yaml` — VPC, RDS PostgreSQL 16, ALB with HTTPS listener, Secrets Manager for all credentials, CloudWatch logging, least-privilege IAM.
- UI container image added to release pipeline (`ghcr.io/<repo>/ui:<tag>`) with multi-arch + SBOM + provenance.
- Helm lint CI workflow at `.github/workflows/helm-lint.yml` — validates chart on PRs.

## Previously Closed (2026-03-04)

- Published operations runbooks at `docs/runbooks/` — backup/restore (`backup-restore.md`), incident response (`incident-response.md`), key rotation (`key-rotation.md`), upgrade/rollback (`upgrade-rollback.md`), and disaster recovery (`disaster-recovery.md`). Runbooks reference actual API endpoints, Prometheus metrics, environment variables, and migration procedures from the codebase.
- Published `docs/openapi.yaml` — OpenAPI 3.1 specification covering all health, setup, public v1, and admin endpoints with security schemes, schemas, and response definitions.
- Published `docs/production-ingress.md` — production ingress/TLS/certificate deployment guide with Nginx, Kubernetes (nginx-ingress + Traefik), AWS ALB, GCP HTTPS LB, cert-manager, security headers, and environment variable reference.
- Added root `SECURITY.md` documenting private vulnerability reporting via GitHub Security Advisories, supported versions, and implemented security controls.
- Added root `LICENSE` with Apache License 2.0 for distribution/legal metadata completeness.
- Removed production migration-path demo defaults and added cleanup migration for legacy demo rows/hashes.
- Implemented setup hardening contract (token-required mode, bearer auth pattern, idempotency key requirement, CIDR network gate, setup state/audit/metrics).
- Added setup smoke gate in CI (`.github/workflows/go.yml`).
- Added marketplace onboarding verification harness and artifact workflow (`scripts/verify_marketplace_onboarding.sh`, `.github/workflows/marketplace-onboarding.yml`).
- Added dedicated release workflow (`.github/workflows/release.yml`) with gated checks and release artifacts (binaries, checksums, onboarding report, optional multi-arch images).

## Exit Criteria to Mark GTM Deployment Ready (Phase 1 — AWS)

All P0 blockers above are closed, and:

- A reproducible release pipeline produces signed artifacts and SBOM.
- A production install path (container/Kubernetes) passes automated smoke tests.
- Security/legal docs are present and published.
- AWS Marketplace packaging/verification artifacts are accepted in pre-submission checks.
- AWS Marketplace Metering API integration is verified in a staging environment.

## Post-Launch Backlog

After AWS Marketplace launch:
- **#14** — Add browser-level UI e2e smoke tests to CI release gates.
- **#16** — Implement GCP Cloud Commerce Partner Procurement API integration.
- Adapt Helm chart for GCP Marketplace container listing requirements.
- Add GCP-specific deployment guidance to `docs/production-ingress.md`.
