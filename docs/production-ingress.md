# Production Ingress, TLS & Certificate Guide

This document provides reference configurations for deploying rbitr behind a TLS-terminating proxy or load balancer in production environments.

## Architecture Overview

rbitr exposes two services — a Go API gateway and an Nginx-based UI — both listening on plain HTTP. TLS terminates at an external proxy or load balancer.

```
                         ┌──────────────────────────┐
    Internet (HTTPS)     │  TLS Termination Proxy   │
  ──────────────────────▶│  (Nginx / ALB / Traefik) │
                         └────────┬─────────┬───────┘
                                  │         │
                         HTTP :8080    HTTP :5173
                                  │         │
                          ┌───────▼──┐  ┌───▼──────┐
                          │ Gateway  │  │  UI      │
                          │ (Go/Echo)│  │ (Nginx)  │
                          └──────────┘  └──────────┘
```

The gateway serves the API (`/admin`, `/setup`, `/v1`, `/api/marketplace`, `/healthz`, `/readyz`, `/metrics`). The UI serves the dashboard SPA and proxies API calls to the gateway via its internal Nginx config.

## Reference Nginx TLS Configuration

Extend the existing `ui/nginx.conf.template` pattern with a TLS server block placed in front of both services.

```nginx
# /etc/nginx/conf.d/rbitr-tls.conf

# Redirect HTTP → HTTPS
server {
    listen 80;
    server_name rbitr.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name rbitr.example.com;

    # --- TLS certificates ---
    ssl_certificate     /etc/ssl/certs/rbitr.example.com.crt;
    ssl_certificate_key /etc/ssl/private/rbitr.example.com.key;

    # --- Protocol & cipher policy ---
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;

    # --- Security headers ---
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'" always;

    # --- API proxy (gateway :8080) ---
    location /admin {
        proxy_pass http://gateway:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /setup {
        proxy_pass http://gateway:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /v1 {
        proxy_pass http://gateway:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE/MCP streaming support — 5 min timeout matches gateway default
        proxy_read_timeout 300s;
        proxy_buffering off;
        proxy_cache off;
    }

    location /api/marketplace {
        proxy_pass http://gateway:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /healthz {
        proxy_pass http://gateway:8080;
        access_log off;
    }

    location /readyz {
        proxy_pass http://gateway:8080;
        access_log off;
    }

    location /metrics {
        proxy_pass http://gateway:8080;
        # Restrict metrics to internal networks
        allow 10.0.0.0/8;
        allow 172.16.0.0/12;
        allow 192.168.0.0/16;
        deny all;
    }

    # --- UI proxy (Nginx :5173) ---
    location / {
        proxy_pass http://ui:5173;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**SSE/MCP note:** The `/v1/mcp/:tenant_id` GET endpoint uses Server-Sent Events with a 5-minute maximum stream duration and 15-second heartbeat interval. Ensure proxy read timeouts are set to at least 300 seconds and buffering is disabled for the `/v1` location.

## Deployment Packages

rbitr provides two pre-built deployment paths for AWS:

- **Helm chart (EKS):** `deploy/helm/rbitr/` — includes ALB ingress, migration job, optional bundled PostgreSQL, HPA, and IRSA-ready ServiceAccount. The chart's `ingress.yaml` template implements the ALB path-based routing described in the AWS ALB section below. Use `values-production.yaml` for HA defaults.
- **CloudFormation (ECS Fargate):** `deploy/cloudformation/rbitr-ecs.yaml` — creates a full VPC, RDS PostgreSQL, ALB with HTTPS listener, and ECS services. The ALB listener rules match the routing pattern described below.

For Kubernetes deployments using the Helm chart, ingress is managed by the chart's `ingress.yaml` template and does not need to be configured separately. The examples below are reference configurations for manual Kubernetes deployments without the Helm chart, or for customization guidance.

## Kubernetes Ingress Examples

### nginx-ingress with cert-manager

```yaml
apiVersion: v1
kind: Service
metadata:
  name: rbitr-gateway
spec:
  selector:
    app: rbitr-gateway
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: rbitr-ui
spec:
  selector:
    app: rbitr-ui
  ports:
    - name: http
      port: 5173
      targetPort: 5173
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: rbitr
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    # SSE streaming support for MCP endpoints
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - rbitr.example.com
      secretName: rbitr-tls
  rules:
    - host: rbitr.example.com
      http:
        paths:
          - path: /admin
            pathType: Prefix
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /setup
            pathType: Prefix
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /v1
            pathType: Prefix
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /api/marketplace
            pathType: Prefix
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /healthz
            pathType: Exact
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /readyz
            pathType: Exact
            backend:
              service:
                name: rbitr-gateway
                port:
                  number: 8080
          - path: /
            pathType: Prefix
            backend:
              service:
                name: rbitr-ui
                port:
                  number: 5173
```

### Traefik IngressRoute

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: rbitr
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`rbitr.example.com`) && (PathPrefix(`/admin`) || PathPrefix(`/setup`) || PathPrefix(`/v1`) || PathPrefix(`/api/marketplace`) || Path(`/healthz`) || Path(`/readyz`))
      kind: Rule
      services:
        - name: rbitr-gateway
          port: 8080
    - match: Host(`rbitr.example.com`)
      kind: Rule
      services:
        - name: rbitr-ui
          port: 5173
  tls:
    certResolver: letsencrypt
```

## Cloud Load Balancer Guidance

### AWS Application Load Balancer (ALB)

1. **Certificate:** Use AWS Certificate Manager (ACM) to provision a public certificate for your domain. ACM certificates auto-renew.
2. **Target groups:**
   - `rbitr-gateway` — port 8080, health check on `GET /healthz` (expected 200).
   - `rbitr-ui` — port 5173, health check on `GET /healthz` (expected 200, served by the UI Nginx).
3. **Listener rules (HTTPS :443):**
   - Path `/admin/*`, `/setup/*`, `/v1/*`, `/api/marketplace/*`, `/healthz`, `/readyz`, `/metrics` → `rbitr-gateway` target group.
   - Default → `rbitr-ui` target group.
4. **HTTP :80 listener:** redirect to HTTPS.
5. **Idle timeout:** Set to 300 seconds for SSE/MCP streaming support.
6. **Stickiness:** Not required — rbitr is stateless at the HTTP layer.

### GCP HTTPS Load Balancer

1. **Certificate:** Use Google-managed SSL certificates for automatic provisioning and renewal.
2. **Backend services:**
   - `rbitr-gateway` — port 8080, health check on `/healthz`.
   - `rbitr-ui` — port 5173, health check on `/healthz`.
3. **URL map:** Route `/admin/*`, `/setup/*`, `/v1/*`, `/api/marketplace/*` to gateway backend. Default to UI backend.
4. **Timeout:** Set backend service timeout to 300 seconds for MCP/SSE endpoints.
5. **HTTP-to-HTTPS redirect:** Configure via a separate HTTP target proxy with redirect action.

## Certificate Management

### cert-manager with Let's Encrypt

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ops@example.com
    privateKeySecretRef:
      name: letsencrypt-prod-account-key
    solvers:
      # HTTP-01 solver (requires ingress to be reachable on port 80)
      - http01:
          ingress:
            ingressClassName: nginx
      # DNS-01 solver (for wildcard certs or private clusters)
      # - dns01:
      #     cloudDNS:
      #       project: my-gcp-project
```

### AWS Certificate Manager (ACM)

- Request a certificate via the ACM console or CLI (`aws acm request-certificate`).
- Validate via DNS (recommended) or email.
- ACM handles renewal automatically for certificates attached to ALB/CloudFront.
- ACM certificates cannot be exported — use only with AWS services.

### GCP Managed Certificates

- Create a `ManagedCertificate` resource or use `gcloud compute ssl-certificates create`.
- Google handles renewal automatically.
- Attach to the HTTPS target proxy in your load balancer configuration.

### Manual Certificate Rotation Checklist

If using self-managed certificates (not recommended for production):

1. Generate new certificate and key pair.
2. Validate the new certificate chain: `openssl verify -CAfile ca.crt new-cert.crt`.
3. Update the TLS secret or certificate file in the proxy/LB.
4. Reload the proxy without downtime: `nginx -s reload` or update the Kubernetes Secret.
5. Verify TLS handshake: `openssl s_client -connect rbitr.example.com:443 -servername rbitr.example.com`.
6. Monitor for TLS errors in proxy access/error logs for 15 minutes.
7. Remove the old certificate after confirming rollover.

## Security Headers

The following headers should be set at the TLS proxy layer (shown in the Nginx reference above):

| Header | Value | Purpose |
|--------|-------|---------|
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains; preload` | Enforce HTTPS for 2 years, eligible for HSTS preload list |
| `X-Content-Type-Options` | `nosniff` | Prevent MIME-type sniffing |
| `X-Frame-Options` | `DENY` | Prevent clickjacking via iframe embedding |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limit referrer information leakage |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'` | Restrict resource loading to same origin |

Adjust the CSP header based on your UI customization requirements.

## Environment Variable Reference

All deployment-relevant environment variables consumed by the gateway (`internal/config/config.go`):

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://postgres@localhost:2345/rbitr?sslmode=require` | PostgreSQL connection string |
| `DB_MAX_OPEN_CONNS` | `30` | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | `10` | Maximum idle database connections |
| `DB_CONN_MAX_LIFETIME_SECONDS` | `1800` (30 min) | Maximum connection lifetime in seconds |
| `DB_CONN_MAX_IDLE_TIME_SECONDS` | `300` (5 min) | Maximum idle connection time in seconds |
| `LISTEN_ADDR` | `:8080` | Gateway listen address |
| `BODY_LIMIT_BYTES` | `262144` (256 KiB) | Maximum request body size |
| `RESPONSE_LIMIT_BYTES` | `262144` (256 KiB) | Maximum upstream response size |
| `RBTR_DISABLE_X_TENANT_KEY` | `false` | Disable deprecated `X-Tenant-Key` header |
| `RBTR_FEATURE_RATE_LIMITING` | `false` | Enable per-tenant/agent/tool rate limiting |
| `RBTR_FEATURE_ARG_CONSTRAINTS` | `false` | Enable argument constraint checking |
| `RBTR_FEATURE_SHADOW_MODE` | `false` | Enable shadow-mode policy evaluation |
| `RBTR_DEV_AUTO_TOOLS` | `false` | Seed dev tools on setup (dev only) |
| `RBTR_DEV_MOCK_INTERNAL_URL` | `http://localhost:8090` | Mock internal tool URL (dev only) |
| `RBTR_DEV_JIRA_URL` | `http://localhost:8081` | Dev Jira tool URL (dev only) |
| `RBTR_SETUP_TOKEN_REQUIRED` | `false` | Require bearer token for `/setup/initialize` |
| `RBTR_SETUP_TOKEN` | _(empty)_ | Setup bearer token value |
| `RBTR_SETUP_TOKEN_FILE` | _(empty)_ | Path to file containing setup token |
| `RBTR_SETUP_ALLOWED_CIDRS` | _(empty)_ | Comma-separated CIDRs allowed for setup requests |

**Production recommendations:**
- Always set `DATABASE_URL` with `sslmode=require` or `sslmode=verify-full`.
- Set `RBTR_SETUP_TOKEN_REQUIRED=true` and provide a strong token via `RBTR_SETUP_TOKEN_FILE` (mounted secret).
- Set `RBTR_SETUP_ALLOWED_CIDRS` to restrict setup access to trusted admin networks.
- Leave `RBTR_DEV_AUTO_TOOLS=false` in production.
