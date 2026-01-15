# Security Review

Date: 2026-01-15

## Threat Model (Brief)
- Assets: shared secret hash, session cookies, device whitelist, device tickets, admin bootstrap token, WS stream.
- Entry points: `/api/device/challenge`, `/api/device/attest`, `/api/login`, `/api/admin/devices`, `/ws`.
- Trust boundaries: Caddy reverse proxy, Origin/CORS, cookies, IP-based rate limits.
- Adversaries: external attacker, malicious client, token theft.

## Device Authorization (Implemented)
- **Flow**: challenge → attest → device_ticket → login → session → ws.
  - `/api/device/challenge` issues `{challenge_id, nonce}` for enrolled devices.
  - `/api/device/attest` verifies ECDSA signature and issues `device_ticket` (HMAC-signed token, version=2).
  - `/api/login` and `/ws` require valid `device_ticket` (and session for `/ws`).
- **Evidence**: `internal/handler/api.go` implements `/api/device/*` and enforces `device_ticket` in login/ws.
- **Command**: `rg -n "device/challenge|device/attest|device_ticket" internal/handler/api.go`
- **Output summary**: challenge/attest handlers exist; login/ws enforce device tickets.

## Session & Token Integrity
- Session tokens use HMAC signatures (TokenVersionSession=1) and are validated with version checks.
- Device tickets use the same signing key but distinct version (TokenVersionDeviceTicket=2) to prevent cross-use.

## Proxy Trust Boundary
- `SetTrustedProxies` only trusts `X-Forwarded-For` when `RemoteAddr` is within configured CIDRs or IPs.
- Env: `TRUSTED_PROXY_CIDRS` (comma-separated CIDR/IPs). `TRUSTED_PROXIES` is accepted as fallback.

## Container Hardening
- Runtime container now runs as non-root user (`fileflow`, UID 10001).

## Production Requirements
- `APP_DOMAIN` required in production
- `SESSION_KEY` required in production (also required when `SECURE_COOKIES=true`)
- `TRUSTED_PROXY_CIDRS` recommended when behind reverse proxy

## Validation Commands
- `go test ./...`
- `go vet ./...`
- `make lint`
