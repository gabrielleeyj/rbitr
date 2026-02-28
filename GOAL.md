# GTM Deployment Readiness Goal

This file tracks **current blockers** preventing rbitr from being GTM deployment-ready for:

- Direct production deployment (container service)
- AWS Marketplace listing
- GCP Marketplace listing

Date updated: 2026-02-28

## P0 Blockers (Must Fix Before Any External Production GTM)

1. Remove seeded static credentials and demo tenant defaults from production migration path.
   - Current state: `tenant_demo_key`, `admin_demo_key`, `t_demo` are seeded in `migrations/00001_init.sql`.
   - Blocker reason: insecure bootstrap pattern; fails production security posture.

2. Replace demo/localhost tool defaults with production-safe bootstrap flow.
   - Current state: seeded tools point to `http://localhost:8090` and `http://localhost:8081`.
   - Blocker reason: deployment is not immediately usable in real environments without unsafe/manual rewiring.

3. Make test/build pipeline reproducible and pinned.
   - Current state: `scripts/test.sh` installs test tooling via `@latest`.
   - Blocker reason: non-deterministic CI/release verification; cannot support repeatable GTM releases.

4. Resolve toolchain/version drift baseline in build/test path.
   - Current state: environment hit `go1.25.7` tool vs `go1.25.6` GOROOT mismatch during `go test ./...`.
   - Blocker reason: no trustworthy release-quality pass/fail baseline until deterministic build toolchain is enforced.

## P0 Marketplace Packaging Blockers (AWS + GCP)

5. Provide production deployment package artifacts.
   - Current state: only `docker-compose.yml` exists; no Helm chart/Kubernetes manifests/Terraform packaging.
   - Blocker reason: marketplace deployment packaging/review paths require standardized deploy artifacts.

6. Add marketplace-compliant integration/verification harness.

- Current state: demo scripts exist, but no marketplace-grade install/upgrade/smoke test package.
- Blocker reason: listing review requires reliable install and runtime verification.

7. Implement commercial entitlement/metering integration for paid marketplace motion.

- Current state: no AWS metering integration and no GCP procurement entitlement integration present.
- Blocker reason: paid marketplace GTM cannot launch without billing/entitlement wiring.

## P0 Security/Compliance Blockers

8. Add security policy and vulnerability disclosure process docs.

- Current state: no `SECURITY.md`.
- Blocker reason: enterprise buyer and marketplace review expectations not met.

9. Add software license/distribution metadata.

- Current state: no root `LICENSE`.
- Blocker reason: legal/commercial packaging incomplete for GTM.

## P1 Blockers (Should Fix Before Broad GTM Scale, But Not Always Day-1 Hard Stop)

11. Complete approval execution idempotency/in-flight ownership hardening.

- Current state: known follow-up notes duplicate side-effect risk during retry windows.
- Risk: correctness and financial/control side effects under race/retry conditions.

12. Add full runbooks for operations.

- Needed: backup/restore, incident response, key rotation, upgrade/rollback, DR.
- Risk: production support model not complete for enterprise onboarding.

13. Add standardized API contract artifacts (OpenAPI/JSON schemas for external consumers).

- Risk: integration friction and higher support burden.

14. Add UI/API e2e smoke tests in CI for release gates.

- Current state: unit/integration coverage exists, but no full-path deployment smoke gate.
- Risk: regression escape risk at release time.

15. Add explicit production ingress/TLS/certificate guidance and reference configs.

- Risk: inconsistent or insecure customer deployments.

## Recently Closed (2026-02-28)

- Switched UI container to a production image path (build-time `vite build` + Nginx static serve) instead of `npm run dev`.
- Removed admin key persistence from browser `localStorage`; admin session key is now in-memory only.
- Enforced secure DB transport default by switching fallback `DATABASE_URL` to `sslmode=require`.
- Added DB pool/concurrency runtime tuning (`max open`, `max idle`, `connection lifetime`, `idle time`) with env-configurable defaults and gateway wiring.
- Added CI security gates with enforced fail policy (SAST/go dependency checks/UI dependency audit/container scan) via `.github/workflows/security.yml`.
- Added SBOM generation and keyless signing/attestation flow for pushed gateway container images (GHCR + cosign + provenance attestation).
- Closed admin key hash hardening gap: admin auth now supports HMAC-based hashes with legacy SHA-256 fallback + lazy upgrade, and setup writes admin hashes through the HMAC path when configured.

## Exit Criteria to Mark GTM Deployment Ready

All P0 blockers above are closed, and:

- A reproducible release pipeline produces signed artifacts and SBOM.
- A production install path (container/Kubernetes) passes automated smoke tests.
- Security/legal docs are present and published.
- Marketplace-specific packaging/verification artifacts are accepted in pre-submission checks.
