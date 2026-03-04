# EPIC 9 — First-Run Setup & Production Bootstrap

## Summary

Epic 9 makes first deployment safe, deterministic, and production-ready.

Current onboarding exists (`/setup/status`, `/setup/initialize`, setup UI), but production hardening is incomplete in concurrency control, setup authorization, resilience, and install verification.

This epic closes those gaps so a fresh container deployment can be brought online reliably with minimal operator error.

Date scoped: 2026-02-28

## Progress Update (2026-03-03)

- P0-6 (initial depth): script-driven setup smoke gate is in CI via `scripts/test_setup_smoke.sh` in `.github/workflows/go.yml`.
- P0-7: onboarding-aware install/upgrade verification harness added:
  - Local script: `scripts/verify_marketplace_onboarding.sh`
  - Manual CI workflow + artifact upload: `.github/workflows/marketplace-onboarding.yml`
  - Machine-readable report artifact: `artifacts/marketplace_onboarding_report.json`
- Release gate depth extension:
  - Dedicated release workflow: `.github/workflows/release.yml`
  - Enforces lint/tests/setup smoke/marketplace onboarding before publishing release artifacts.

## Current Baseline (Implemented)

- Setup API:
  - `GET /setup/status`
  - `POST /setup/initialize`
- Setup UI wizard:
  - Welcome -> Environment checks -> Configure tenant/keys -> Complete
- Setup creates:
  - First tenant
  - First tenant key
  - First admin key
  - Default policy and active tenant config
  - Bootstrap settings
- Dev-only auto tool wiring is available via:
  - `RBTR_DEV_AUTO_TOOLS=true`
  - `RBTR_DEV_MOCK_INTERNAL_URL`
  - `RBTR_DEV_JIRA_URL`

## Production Gaps to Close

1. Setup race safety and idempotency are not hardened.
2. Setup endpoint authorization is open pre-bootstrap (no bootstrap token or equivalent gate).
3. No recovery contract for partial/timeout/retry bootstrap workflows.
4. Setup test depth is shallow (handler tests only; no DB-backed service tests/e2e coverage).
5. Setup observability is limited (no dedicated metrics/audit trail for setup lifecycle).
6. No production install verification harness specifically validating first-run onboarding path.
7. DB configuration UX expectation needs explicit decision:
   - Production: DB URL is deployment-time env (recommended).
   - Dev convenience should remain optional and explicit.

## Scope (Stories)

### P0-1: Bootstrap Concurrency + Idempotency Hardening

Deliverables:
- Add a global setup lock (advisory lock or transactional compare-and-set).
- Ensure only one successful bootstrap transaction can commit.
- Add idempotency support for initialize retries (request token + persisted result envelope).

Acceptance criteria:
- Parallel initialize requests result in exactly one successful bootstrap.
- Retry after client timeout does not create duplicate tenants/keys.
- Automated test covers concurrent initialize requests.

### P0-2: Setup Access Control

Deliverables:
- Require a bootstrap setup token for `POST /setup/initialize` in production mode.
- Add explicit config for setup exposure policy:
  - enabled only until bootstrap complete
  - optional network/host restrictions
- Return clear 401/403/409 error semantics.

Acceptance criteria:
- Initialize is rejected without valid setup token when production mode is enabled.
- Setup endpoint cannot be abused after bootstrap complete.

### P0-3: Resumable/Recoverable Setup Contract

Deliverables:
- Add setup state model (`not_started|in_progress|completed|failed`).
- Persist progress and last error surface for operator troubleshooting.
- Add safe resume behavior if process restarts mid-setup.

Acceptance criteria:
- Restart during setup does not leave ambiguous state.
- UI can recover and continue or safely restart setup.

### P0-4: Production-safe Setup Inputs and Validation

Deliverables:
- Harden key input policy (entropy/format checks for user-supplied keys).
- Validate tenant id/name uniqueness and format with explicit errors.
- Clarify DB handling:
  - production uses `DATABASE_URL` provided at deploy time
  - setup validates connectivity/migrations, does not mutate runtime DB config.

Acceptance criteria:
- Invalid setup inputs return deterministic field-level errors.
- Setup UX clearly distinguishes deploy-time infra config vs setup-time tenant bootstrap.

### P0-5: Setup Lifecycle Observability + Audit

Deliverables:
- Add structured logs for setup start/success/failure.
- Add metrics:
  - `setup_attempts_total{result}`
  - `setup_duration_ms`
  - `setup_state`
- Emit immutable admin audit events for setup lifecycle and initial key creation.

Acceptance criteria:
- Operators can answer: who ran setup, when, outcome, and duration.

### P0-6: Setup Service Test Suite + End-to-End Gate

Deliverables:
- Add DB-backed tests for `internal/api/setup/service.go`.
- Add integration test for:
  - schema not ready
  - successful initialize
  - already initialized conflict
  - parallel initialize race behavior
- Add UI/API smoke test in CI for first-run onboarding flow.

Acceptance criteria:
- CI blocks release on setup regressions.
- Setup flows are covered by automated tests, not manual validation.

### P0-7: Marketplace Install/Upgrade Verification Harness (Onboarding-aware)

Deliverables:
- Add a repeatable harness that validates:
  - fresh install -> setup -> operational admin/API calls
  - upgrade preserving bootstrap state
- Produce machine-readable test report artifact.

Acceptance criteria:
- AWS/GCP packaging review can use deterministic install verification outputs.

## Recommended Explicit Decisions

1. DB URL in setup UI:
   - Recommended: do not configure DB URL from UI in production.
   - Keep DB URL as deployment-time immutable runtime config.
2. No-DB fallback:
   - Recommended: support only for dev/local profiles, not production GTM.
3. Setup endpoint lifecycle:
   - Keep status endpoint readable.
   - Restrict initialize endpoint by token/policy until bootstrap completes.

## Non-goals for Epic 9

- Full identity/SSO onboarding flow.
- Full tenant import/migration wizard.
- Marketplace entitlement/metering logic (separate GTM epic).

## Definition of Done

- All P0 stories above are complete with tests.
- Setup is safe under concurrency and restart.
- Setup is authorization-gated in production.
- CI includes setup e2e smoke gate.
- Install/upgrade verification harness validates first-run onboarding path.

## Execution Order

1. P0-1 Bootstrap concurrency/idempotency.
2. P0-2 Setup access control.
3. P0-3 Resumable state model.
4. P0-4 Input/UX hardening.
5. P0-5 Observability/audit.
6. P0-6 Test suite + CI gate.
7. P0-7 Marketplace verification harness.
