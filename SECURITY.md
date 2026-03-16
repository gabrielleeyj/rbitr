# Security Policy

## Reporting a Vulnerability

- Use GitHub Security Advisories (private vulnerability reporting).
- Report here: <https://github.com/gabrielleeyj/rbitr/security/advisories/new>
- Expected response time: 48-hour acknowledgment, 90-day coordinated disclosure window.
- Do not open public issues for security vulnerabilities.

## Supported Versions

| Version | Supported |
| --- | --- |
| latest release | Yes |
| older releases | Best-effort |

## Security Practices

### Build and Supply Chain

- Distroless container base (`gcr.io/distroless/base-debian12`).
- Non-root container execution (`USER nonroot:nonroot`).
- Multi-stage builds with stripped binaries (`CGO_ENABLED=0`, `-ldflags "-s -w"`).
- SBOM generation and provenance attestations for release images published to GHCR.
- SHA256 checksums for release binary archives.

### Static Analysis and Scanning

- `golangci-lint` with `gosec`, `govet`, and `staticcheck` on every PR.
- `gitleaks` secret scanning in CI.

### Authentication and Secrets

- HMAC-SHA256 key hashing for admin and tenant keys.
- Ephemeral session tokens (HMAC-SHA256, 15-min TTL, IP-bound) for MCP sessions.
- Setup token enforcement with optional CIDR network gating.
- Key rotation support with previous-key fallback candidates.
- No default/demo credentials in the production setup path.

### Agent-to-Agent Security

- Mandatory base policy layer prevents tenant policies from overriding DENY/REQUIRE_APPROVAL for critical actions.
- File access governance detects file paths in tool arguments and enforces per-tenant sandboxing.
- Path traversal detection blocks `../` sequences at the gateway before tool execution.
- Cross-tenant provenance chain tracks request origin across tenant boundaries with signed tokens and ADR linkage.
- Chain depth limiting (default 5 hops) prevents infinite cross-tenant request loops.

### Audit and Compliance

- Append-only, hash-chained admin audit trails.
- Configurable audit retention (`audit_retention_days`, default `365`).
- Structured setup/admin audit metadata with request context.
- Audit export (`JSON`/`CSV`) with redaction of sensitive values.

## Dependency Security

- Go module dependencies are integrity-locked via `go.sum`.
- Report dependency vulnerabilities via GitHub Security Advisories.
